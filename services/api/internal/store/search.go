package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AssetFilter holds the structured (non-AI) search filters for the timeline.
type AssetFilter struct {
	From      *time.Time
	To        *time.Time
	MediaType string  // "photo" | "video" | ""
	Favorite  *bool
	AlbumID   string
	DeviceID  string
	Limit     int
	Before    *time.Time // keyset cursor
	Offset    int        // page offset (simple pagination, robust to null captured_at)
	Ascending bool       // oldest-first when true (default newest-first)
}

// ListAssetsFiltered returns completed assets matching the filter, newest-first,
// keyset-paginated by (captured_at, id).
func (s *Store) ListAssetsFiltered(ctx context.Context, userID string, f AssetFilter) ([]TimelineAsset, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 60
	}

	var conds []string
	var args []any
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	join := ""
	args = append(args, userID) // $1
	conds = append(conds, "a.user_id = $1")
	conds = append(conds, "a.status = 'complete'")
	conds = append(conds, "a.deleted_at IS NULL")

	if f.AlbumID != "" {
		args = append(args, f.AlbumID)
		join = fmt.Sprintf(" JOIN album_assets aa ON aa.asset_id = a.id AND aa.album_id = $%d", len(args))
	}
	if f.MediaType != "" {
		add("a.media_type = $%d", f.MediaType)
	}
	if f.Favorite != nil {
		add("a.favorite = $%d", *f.Favorite)
	}
	if f.From != nil {
		add("a.captured_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("a.captured_at < $%d", *f.To)
	}
	if f.Before != nil {
		add("a.captured_at < $%d", *f.Before)
	}
	if f.DeviceID != "" {
		args = append(args, f.DeviceID)
		conds = append(conds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM uploads u WHERE u.asset_id = a.id AND u.device_id = $%d)", len(args)))
	}

	dir := "DESC"
	if f.Ascending {
		dir = "ASC"
	}
	q := `SELECT a.id, a.media_type, a.byte_size, a.width, a.height, a.captured_at, a.status, a.favorite, a.storage_key, a.thumb_key
	      FROM assets a` + join +
		` WHERE ` + strings.Join(conds, " AND ") +
		` ORDER BY a.captured_at ` + dir + ` NULLS LAST, a.id ` + dir + ` LIMIT ` + itoa(limit)
	if f.Offset > 0 {
		q += ` OFFSET ` + itoa(f.Offset)
	}

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

type DeviceFacet struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Count    int64  `json:"count"`
}

type Facets struct {
	PhotoCount    int64         `json:"photo_count"`
	VideoCount    int64         `json:"video_count"`
	FavoriteCount int64         `json:"favorite_count"`
	Total         int64         `json:"total"`
	Devices       []DeviceFacet `json:"devices"`
}

// FacetCounts returns filter-bar counts: per media type, favorites, total, per device.
func (s *Store) FacetCounts(ctx context.Context, userID string) (Facets, error) {
	var f Facets
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE media_type='photo'),
		  COUNT(*) FILTER (WHERE media_type='video'),
		  COUNT(*) FILTER (WHERE favorite),
		  COUNT(*)
		FROM assets WHERE user_id=$1 AND status='complete' AND deleted_at IS NULL`, userID,
	).Scan(&f.PhotoCount, &f.VideoCount, &f.FavoriteCount, &f.Total)
	if err != nil {
		return f, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, COUNT(DISTINCT a.id)
		FROM devices d
		JOIN uploads u ON u.device_id = d.id
		JOIN assets a ON a.id = u.asset_id AND a.status='complete' AND a.deleted_at IS NULL
		WHERE d.user_id=$1
		GROUP BY d.id, d.name
		ORDER BY 3 DESC`, userID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	f.Devices = []DeviceFacet{}
	for rows.Next() {
		var d DeviceFacet
		if err := rows.Scan(&d.DeviceID, &d.Name, &d.Count); err != nil {
			return f, err
		}
		f.Devices = append(f.Devices, d)
	}
	return f, rows.Err()
}
