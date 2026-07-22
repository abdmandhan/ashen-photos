package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Asset struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	SHA256     string     `json:"sha256"`
	MediaType  string     `json:"media_type"`
	ByteSize   int64      `json:"byte_size"`
	CapturedAt       *time.Time `json:"captured_at"`
	StorageKey       string     `json:"storage_key"`
	LivePhotoGroupID *string    `json:"live_photo_group_id"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Upload struct {
	ID        string    `json:"id"`
	AssetID   string    `json:"asset_id"`
	DeviceID  *string   `json:"device_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ExistingHashes returns the subset of sha256 values that already have a
// completed-or-in-flight asset for this user (dedup check).
func (s *Store) ExistingHashes(ctx context.Context, userID string, hashes []string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sha256, id FROM assets WHERE user_id=$1 AND sha256 = ANY($2)`,
		userID, hashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var h, id string
		if err := rows.Scan(&h, &id); err != nil {
			return nil, err
		}
		out[h] = id
	}
	return out, rows.Err()
}

// CreateAsset inserts a pending asset. If one already exists for (user, sha256)
// it returns the existing row so uploads are idempotent.
func (s *Store) CreateAsset(ctx context.Context, a Asset) (Asset, error) {
	var out Asset
	err := s.pool.QueryRow(ctx,
		`INSERT INTO assets(user_id, sha256, media_type, byte_size, captured_at, storage_key, live_photo_group_id, status)
		 VALUES($1,$2,$3,$4,$5,$6,$7,'pending')
		 ON CONFLICT (user_id, sha256) DO UPDATE SET sha256 = EXCLUDED.sha256
		 RETURNING id, user_id, sha256, media_type, byte_size, captured_at, storage_key, live_photo_group_id, status, created_at`,
		a.UserID, a.SHA256, a.MediaType, a.ByteSize, a.CapturedAt, a.StorageKey, a.LivePhotoGroupID,
	).Scan(&out.ID, &out.UserID, &out.SHA256, &out.MediaType, &out.ByteSize,
		&out.CapturedAt, &out.StorageKey, &out.LivePhotoGroupID, &out.Status, &out.CreatedAt)
	return out, err
}

func (s *Store) CreateUpload(ctx context.Context, assetID string, deviceID *string) (Upload, error) {
	var u Upload
	err := s.pool.QueryRow(ctx,
		`INSERT INTO uploads(asset_id, device_id, status) VALUES($1,$2,'pending')
		 RETURNING id, asset_id, device_id, status, created_at`,
		assetID, deviceID,
	).Scan(&u.ID, &u.AssetID, &u.DeviceID, &u.Status, &u.CreatedAt)
	return u, err
}

// UploadForComplete loads the join needed to enqueue a verify job, scoped to the user.
type UploadDetail struct {
	UploadID   string
	AssetID    string
	UserID     string
	SHA256     string
	MediaType  string
	StorageKey string
}

func (s *Store) UploadDetail(ctx context.Context, userID, uploadID string) (UploadDetail, error) {
	var d UploadDetail
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, a.id, a.user_id, a.sha256, a.media_type, a.storage_key
		 FROM uploads u JOIN assets a ON a.id = u.asset_id
		 WHERE u.id=$1 AND a.user_id=$2`,
		uploadID, userID,
	).Scan(&d.UploadID, &d.AssetID, &d.UserID, &d.SHA256, &d.MediaType, &d.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// MarkUploaded flips the upload + asset to 'uploaded' in one transaction.
func (s *Store) MarkUploaded(ctx context.Context, uploadID, assetID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE uploads SET status='uploaded', updated_at=now() WHERE id=$1`, uploadID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE assets SET status='uploaded' WHERE id=$1`, assetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
