package store

import "context"

// DupGroup is a set of near-duplicate assets sharing a dup_group_id.
type DupGroup struct {
	GroupID string          `json:"group_id"`
	Assets  []TimelineAsset `json:"assets"`
}

// DuplicateGroups returns the user's near-duplicate groups (2+ live members each),
// newest-first within each group.
func (s *Store) DuplicateGroups(ctx context.Context, userID string) ([]DupGroup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dup_group_id, id, media_type, byte_size, width, height, captured_at, status, favorite, storage_key, thumb_key
		FROM assets
		WHERE user_id=$1 AND dup_group_id IS NOT NULL AND deleted_at IS NULL AND status='complete'
		  AND dup_group_id IN (
		      SELECT dup_group_id FROM assets
		      WHERE user_id=$1 AND dup_group_id IS NOT NULL AND deleted_at IS NULL AND status='complete'
		      GROUP BY dup_group_id HAVING COUNT(*) > 1
		  )
		ORDER BY dup_group_id, captured_at DESC NULLS LAST, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []DupGroup
	var cur *DupGroup
	for rows.Next() {
		var gid string
		var a TimelineAsset
		if err := rows.Scan(&gid, &a.ID, &a.MediaType, &a.ByteSize, &a.Width, &a.Height,
			&a.CapturedAt, &a.Status, &a.Favorite, &a.StorageKey, &a.ThumbKey); err != nil {
			return nil, err
		}
		if cur == nil || cur.GroupID != gid {
			groups = append(groups, DupGroup{GroupID: gid})
			cur = &groups[len(groups)-1]
		}
		cur.Assets = append(cur.Assets, a)
	}
	return groups, rows.Err()
}

// ResolveDuplicate acts on one asset in a dup group.
//   - "delete": soft-delete it (deleted_at = now).
//   - "keep":   dismiss it from the group (dup_group_id = NULL).
func (s *Store) ResolveDuplicate(ctx context.Context, userID, assetID, action string) error {
	var q string
	switch action {
	case "delete":
		q = `UPDATE assets SET deleted_at=now(), dup_group_id=NULL WHERE id=$1 AND user_id=$2`
	case "keep":
		q = `UPDATE assets SET dup_group_id=NULL WHERE id=$1 AND user_id=$2`
	default:
		return ErrNotFound
	}
	ct, err := s.pool.Exec(ctx, q, assetID, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
