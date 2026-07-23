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
	"math/bits"
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

	// Optional secondary replication target.
	replica       *minio.Client
	replicaTarget string
	replicaPhotos string
	replicaVideos string
}

func New(pool *pgxpool.Pool, s3 *minio.Client, thumbBucket string) *Processor {
	return &Processor{pool: pool, s3: s3, thumbBucket: thumbBucket}
}

// WithReplica configures the secondary storage target for replication.
func (p *Processor) WithReplica(replica *minio.Client, target, photos, videos string) *Processor {
	p.replica = replica
	p.replicaTarget = target
	p.replicaPhotos = photos
	p.replicaVideos = videos
	return p
}

func (p *Processor) ReplicationEnabled() bool { return p.replica != nil }

func (p *Processor) replicaBucketFor(mediaType string) string {
	if mediaType == "video" {
		return p.replicaVideos
	}
	return p.replicaPhotos
}

// Replicate streams an object from the primary target to the secondary and
// records the result. Idempotent: an already-replicated object is a no-op.
func (p *Processor) Replicate(ctx context.Context, j job.ReplicateJob) error {
	if p.replica == nil {
		return nil
	}
	target := p.replicaTarget
	dstBucket := p.replicaBucketFor(j.MediaType)

	src, err := p.s3.GetObject(ctx, j.Bucket, j.StorageKey, minio.GetObjectOptions{})
	if err != nil {
		return p.recordReplica(ctx, j.AssetID, target, "failed", err.Error())
	}
	defer src.Close()
	stat, err := src.Stat()
	if err != nil {
		return p.recordReplica(ctx, j.AssetID, target, "failed", err.Error())
	}

	info, err := p.replica.PutObject(ctx, dstBucket, j.StorageKey, src, stat.Size,
		minio.PutObjectOptions{ContentType: stat.ContentType})
	if err != nil {
		return p.recordReplica(ctx, j.AssetID, target, "failed", err.Error())
	}
	// Verify via the size PutObject reports (avoids a separate HEAD that some
	// reverse proxies reject).
	if info.Size != stat.Size {
		return p.recordReplica(ctx, j.AssetID, target, "failed", "size mismatch")
	}
	log.Printf("replicated asset=%s -> %s/%s", j.AssetID, dstBucket, j.StorageKey)
	return p.recordReplica(ctx, j.AssetID, target, "replicated", "")
}

func (p *Processor) recordReplica(ctx context.Context, assetID, target, status, errMsg string) error {
	var replicatedAt *time.Time
	if status == "replicated" {
		now := time.Now()
		replicatedAt = &now
	}
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO asset_replicas(asset_id, target, status, error, replicated_at)
		 VALUES($1,$2,$3,$4,$5)
		 ON CONFLICT (asset_id, target)
		 DO UPDATE SET status=EXCLUDED.status, error=EXCLUDED.error, replicated_at=EXCLUDED.replicated_at`,
		assetID, target, status, e, replicatedAt)
	if status == "failed" && err == nil {
		return nil // recorded the failure; don't propagate (avoids infinite requeue)
	}
	return err
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

	var phash *int64

	// Client-provided thumbnail (HEIC/video decoded natively on device) takes
	// priority — the worker can't decode those formats itself.
	if j.ThumbKey != "" {
		thumbKey = j.ThumbKey
	}

	if j.MediaType == "photo" {
		raw := buf.Bytes()
		if img, _, derr := image.Decode(bytes.NewReader(raw)); derr == nil {
			width = img.Bounds().Dx()
			height = img.Bounds().Dy()
			h := int64(dHash(img)) // perceptual hash for near-dup detection
			phash = &h
			if thumbKey == "" { // no client thumb → generate from decodable formats
				if key, terr := p.makeThumb(ctx, j, img); terr == nil {
					thumbKey = key
				} else {
					log.Printf("thumb asset=%s: %v", j.AssetID, terr)
				}
			}
		} else {
			// HEIC etc: can't decode server-side; rely on the client thumbnail.
			log.Printf("decode asset=%s: %v (using client thumb=%v)", j.AssetID, derr, j.ThumbKey != "")
		}
		capturedAt, exifJSON = extractExif(raw)
	}

	if err := p.markComplete(ctx, j, width, height, thumbKey, capturedAt, exifJSON, phash); err != nil {
		return err
	}
	// Group with visually-similar assets (best-effort; failure doesn't fail the job).
	if phash != nil {
		if err := p.groupDuplicates(ctx, j.UserID, j.AssetID, *phash, width, height); err != nil {
			log.Printf("dedup asset=%s: %v", j.AssetID, err)
		}
	}
	return nil
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

func (p *Processor) markComplete(ctx context.Context, j job.VerifyJob, w, h int, thumbKey string, capturedAt *time.Time, exifJSON []byte, phash *int64) error {
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
		   exif = COALESCE($6, exif),
		   phash = $7
		 WHERE id=$1`,
		j.AssetID, w, h, thumbKey, capturedAt, exifJSON, phash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE uploads SET status='uploaded', updated_at=now() WHERE id=$1`, j.UploadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const hammingThreshold = 10 // max differing bits to count as a near-duplicate

// dHash computes a 64-bit difference hash: grayscale → 9x8 → compare adjacent
// columns. Robust to re-encoding/resizing/minor edits.
func dHash(img image.Image) uint64 {
	small := imaging.Resize(imaging.Grayscale(img), 9, 8, imaging.Lanczos)
	var hash uint64
	bit := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			l, _, _, _ := small.At(x, y).RGBA()
			r, _, _, _ := small.At(x+1, y).RGBA()
			if l > r {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}
	return hash
}

// groupDuplicates finds visually-similar assets (same dimensions, Hamming
// distance within threshold) and assigns them a shared dup_group_id.
func (p *Processor) groupDuplicates(ctx context.Context, userID, assetID string, phash int64, w, h int) error {
	if w == 0 || h == 0 {
		return nil
	}
	rows, err := p.pool.Query(ctx,
		`SELECT id, phash, dup_group_id FROM assets
		 WHERE user_id=$1 AND id<>$2 AND status='complete' AND deleted_at IS NULL
		   AND phash IS NOT NULL AND width=$3 AND height=$4`,
		userID, assetID, w, h)
	if err != nil {
		return err
	}
	defer rows.Close()

	var matchIDs []string
	var existingGroup *string
	for rows.Next() {
		var id string
		var candHash int64
		var group *string
		if err := rows.Scan(&id, &candHash, &group); err != nil {
			return err
		}
		if hamming(uint64(phash), uint64(candHash)) <= hammingThreshold {
			matchIDs = append(matchIDs, id)
			if group != nil && existingGroup == nil {
				existingGroup = group
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(matchIDs) == 0 {
		return nil
	}

	// Join an existing group if a match already has one, else mint a new group.
	var groupID string
	if existingGroup != nil {
		groupID = *existingGroup
	} else {
		if err := p.pool.QueryRow(ctx, `SELECT gen_random_uuid()`).Scan(&groupID); err != nil {
			return err
		}
	}

	ids := append(matchIDs, assetID)
	_, err = p.pool.Exec(ctx,
		`UPDATE assets SET dup_group_id=$1 WHERE id = ANY($2) AND dup_group_id IS NULL`,
		groupID, ids)
	return err
}

func hamming(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
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
