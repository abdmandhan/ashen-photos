package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type TimelineAsset struct {
	ID         string     `json:"id"`
	MediaType  string     `json:"media_type"`
	ByteSize   int64      `json:"byte_size"`
	Width      *int       `json:"width"`
	Height     *int       `json:"height"`
	CapturedAt *time.Time `json:"captured_at"`
	Status     string     `json:"status"`
	Favorite   bool       `json:"favorite"`
	StorageKey string     `json:"-"`
	ThumbKey   *string    `json:"-"`
}

// ListAssets returns completed assets newest-first, keyset-paginated by (captured_at, id).
func (s *Store) ListAssets(ctx context.Context, userID string, limit int, before *time.Time) ([]TimelineAsset, error) {
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	q := `SELECT id, media_type, byte_size, width, height, captured_at, status, favorite, storage_key, thumb_key
	      FROM assets
	      WHERE user_id=$1 AND status='complete'`
	args := []any{userID}
	if before != nil {
		q += ` AND captured_at < $2`
		args = append(args, *before)
	}
	q += ` ORDER BY captured_at DESC NULLS LAST, id DESC LIMIT ` + itoa(limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimelineAsset
	for rows.Next() {
		var a TimelineAsset
		if err := rows.Scan(&a.ID, &a.MediaType, &a.ByteSize, &a.Width, &a.Height,
			&a.CapturedAt, &a.Status, &a.Favorite, &a.StorageKey, &a.ThumbKey); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssetForDownload returns the storage key + media type for a single owned asset.
func (s *Store) AssetForDownload(ctx context.Context, userID, id string) (mediaType, storageKey string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT media_type, storage_key FROM assets WHERE id=$1 AND user_id=$2`, id, userID,
	).Scan(&mediaType, &storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return
}

type Stats struct {
	PhotoCount int64 `json:"photo_count"`
	VideoCount int64 `json:"video_count"`
	TotalBytes int64 `json:"total_bytes"`
}

func (s *Store) Stats(ctx context.Context, userID string) (Stats, error) {
	var st Stats
	err := s.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE media_type='photo' AND status='complete'),
		   COUNT(*) FILTER (WHERE media_type='video' AND status='complete'),
		   COALESCE(SUM(byte_size) FILTER (WHERE status='complete'), 0)
		 FROM assets WHERE user_id=$1`, userID,
	).Scan(&st.PhotoCount, &st.VideoCount, &st.TotalBytes)
	return st, err
}

// itoa avoids importing strconv just for a bounded, validated int.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
