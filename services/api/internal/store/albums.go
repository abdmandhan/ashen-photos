package store

import (
	"context"
	"time"
)

type Album struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Name         string    `json:"name"`
	CoverAssetID *string   `json:"cover_asset_id"`
	AssetCount   int       `json:"asset_count"`
	CoverThumb   *string   `json:"-"` // storage thumb key of the cover, presigned by the handler
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) CreateAlbum(ctx context.Context, userID, name string) (Album, error) {
	var a Album
	err := s.pool.QueryRow(ctx,
		`INSERT INTO albums(user_id, name) VALUES($1,$2)
		 RETURNING id, user_id, name, cover_asset_id, created_at, updated_at`,
		userID, name,
	).Scan(&a.ID, &a.UserID, &a.Name, &a.CoverAssetID, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// ListAlbums returns the user's albums with asset counts and a cover thumbnail
// (explicit cover, else the most-recently-added member's thumb).
func (s *Store) ListAlbums(ctx context.Context, userID string) ([]Album, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT al.id, al.user_id, al.name, al.cover_asset_id, al.created_at, al.updated_at,
		       COUNT(aa.asset_id) AS asset_count,
		       COALESCE(
		           (SELECT a.thumb_key FROM assets a WHERE a.id = al.cover_asset_id),
		           (SELECT a2.thumb_key FROM album_assets aa2
		              JOIN assets a2 ON a2.id = aa2.asset_id
		             WHERE aa2.album_id = al.id AND a2.thumb_key IS NOT NULL
		             ORDER BY aa2.added_at DESC LIMIT 1)
		       ) AS cover_thumb
		FROM albums al
		LEFT JOIN album_assets aa ON aa.album_id = al.id
		WHERE al.user_id = $1
		GROUP BY al.id
		ORDER BY al.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.CoverAssetID,
			&a.CreatedAt, &a.UpdatedAt, &a.AssetCount, &a.CoverThumb); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAlbum(ctx context.Context, userID, id string, name *string, coverAssetID *string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE albums SET
		  name = COALESCE($3, name),
		  cover_asset_id = COALESCE($4, cover_asset_id),
		  updated_at = now()
		WHERE id = $1 AND user_id = $2`, id, userID, name, coverAssetID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAlbum(ctx context.Context, userID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM albums WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ownsAlbum verifies the album belongs to the user (guards membership edits).
func (s *Store) ownsAlbum(ctx context.Context, userID, albumID string) error {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM albums WHERE id=$1 AND user_id=$2)`, albumID, userID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddAssetToAlbum(ctx context.Context, userID, albumID, assetID string) error {
	if err := s.ownsAlbum(ctx, userID, albumID); err != nil {
		return err
	}
	// Ensure the asset is the user's too.
	var owned bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets WHERE id=$1 AND user_id=$2)`, assetID, userID,
	).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO album_assets(album_id, asset_id) VALUES($1,$2)
		 ON CONFLICT DO NOTHING`, albumID, assetID)
	if err == nil {
		_, _ = s.pool.Exec(ctx, `UPDATE albums SET updated_at=now() WHERE id=$1`, albumID)
	}
	return err
}

func (s *Store) RemoveAssetFromAlbum(ctx context.Context, userID, albumID, assetID string) error {
	if err := s.ownsAlbum(ctx, userID, albumID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`DELETE FROM album_assets WHERE album_id=$1 AND asset_id=$2`, albumID, assetID)
	return err
}

// ListAlbumAssets returns the album's completed assets, newest-first.
func (s *Store) ListAlbumAssets(ctx context.Context, userID, albumID string, limit int, before *time.Time) ([]TimelineAsset, error) {
	if err := s.ownsAlbum(ctx, userID, albumID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	q := `SELECT a.id, a.media_type, a.byte_size, a.width, a.height, a.captured_at, a.status, a.favorite, a.storage_key, a.thumb_key
	      FROM album_assets aa JOIN assets a ON a.id = aa.asset_id
	      WHERE aa.album_id = $1 AND a.status = 'complete' AND a.deleted_at IS NULL`
	args := []any{albumID}
	if before != nil {
		q += ` AND a.captured_at < $2`
		args = append(args, *before)
	}
	q += ` ORDER BY a.captured_at DESC NULLS LAST, a.id DESC LIMIT ` + itoa(limit)

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

// SetFavorite toggles an asset's favorite flag.
func (s *Store) SetFavorite(ctx context.Context, userID, assetID string, favorite bool) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE assets SET favorite=$3 WHERE id=$1 AND user_id=$2`, assetID, userID, favorite)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
