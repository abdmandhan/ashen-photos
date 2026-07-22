package processor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"time"

	"github.com/disintegration/imaging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/rwcarlsen/goexif/exif"

	"ashen/worker/internal/job"
)

const thumbLongEdge = 512

type Processor struct {
	pool        *pgxpool.Pool
	s3          *minio.Client
	thumbBucket string
}

func New(pool *pgxpool.Pool, s3 *minio.Client, thumbBucket string) *Processor {
	return &Processor{pool: pool, s3: s3, thumbBucket: thumbBucket}
}

// Process verifies one uploaded object end to end.
func (p *Processor) Process(ctx context.Context, j job.VerifyJob) error {
	obj, err := p.s3.GetObject(ctx, j.Bucket, j.StorageKey, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	h := sha256.New()
	var buf bytes.Buffer // only filled for photos (needed for thumb/exif)
	var dst io.Writer = h
	if j.MediaType == "photo" {
		dst = io.MultiWriter(h, &buf)
	}
	if _, err := io.Copy(dst, obj); err != nil {
		return fmt.Errorf("read object: %w", err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if sum != j.SHA256 {
		log.Printf("checksum mismatch asset=%s want=%s got=%s", j.AssetID, j.SHA256, sum)
		_ = p.s3.RemoveObject(ctx, j.Bucket, j.StorageKey, minio.RemoveObjectOptions{})
		return p.markFailed(ctx, j)
	}

	var (
		width, height int
		thumbKey      string
		capturedAt    *time.Time
		exifJSON      []byte
	)

	if j.MediaType == "photo" {
		raw := buf.Bytes()
		if img, _, derr := image.Decode(bytes.NewReader(raw)); derr == nil {
			width = img.Bounds().Dx()
			height = img.Bounds().Dy()
			if key, terr := p.makeThumb(ctx, j, img); terr == nil {
				thumbKey = key
			} else {
				log.Printf("thumb asset=%s: %v", j.AssetID, terr)
			}
		} else {
			// HEIC and other undecodable formats: verified but no thumb (deferred).
			log.Printf("decode asset=%s: %v (skip thumb)", j.AssetID, derr)
		}
		capturedAt, exifJSON = extractExif(raw)
	}

	return p.markComplete(ctx, j, width, height, thumbKey, capturedAt, exifJSON)
}

func (p *Processor) makeThumb(ctx context.Context, j job.VerifyJob, img image.Image) (string, error) {
	thumb := imaging.Fit(img, thumbLongEdge, thumbLongEdge, imaging.Lanczos)
	var out bytes.Buffer
	if err := imaging.Encode(&out, thumb, imaging.JPEG); err != nil {
		return "", err
	}
	key := j.UserID + "/" + j.SHA256 + ".jpg"
	_, err := p.s3.PutObject(ctx, p.thumbBucket, key, &out, int64(out.Len()),
		minio.PutObjectOptions{ContentType: "image/jpeg"})
	if err != nil {
		return "", err
	}
	return key, nil
}

func extractExif(raw []byte) (*time.Time, []byte) {
	x, err := exif.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil
	}
	fields := map[string]string{}
	for _, name := range []exif.FieldName{exif.DateTimeOriginal, exif.Make, exif.Model, exif.PixelXDimension, exif.PixelYDimension} {
		if t, err := x.Get(name); err == nil {
			fields[string(name)] = t.String()
		}
	}
	var captured *time.Time
	if t, err := x.DateTime(); err == nil {
		captured = &t
	}
	if len(fields) == 0 {
		return captured, nil
	}
	b, _ := json.Marshal(fields)
	return captured, b
}

func (p *Processor) markComplete(ctx context.Context, j job.VerifyJob, w, h int, thumbKey string, capturedAt *time.Time, exifJSON []byte) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE assets SET status='complete',
		   width = NULLIF($2,0), height = NULLIF($3,0),
		   thumb_key = NULLIF($4,''),
		   captured_at = COALESCE(captured_at, $5),
		   exif = COALESCE($6, exif)
		 WHERE id=$1`,
		j.AssetID, w, h, thumbKey, capturedAt, exifJSON); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE uploads SET status='uploaded', updated_at=now() WHERE id=$1`, j.UploadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) markFailed(ctx context.Context, j job.VerifyJob) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE assets SET status='failed' WHERE id=$1`, j.AssetID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE uploads SET status='failed', updated_at=now() WHERE id=$1`, j.UploadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
