package store

import "context"

// DupGroup is a set of near-duplicate assets sharing a dup_group_id.
type DupGroup struct {
	GroupID string          `json:"group_id"`
	Assets  []TimelineAsset `json:"assets"`
}

// DuplicateGroups returns a page of the user's near-duplicate groups (2+ live
// members each), members newest-first within each group. Groups are ordered by
// their most recent backup time (MAX created_at); `ascending` flips oldest-first.
// It also returns the total number of qualifying groups for pagination.
func (s *Store) DuplicateGroups(ctx context.Context, userID string, limit, offset int, ascending bool) ([]DupGroup, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// Total qualifying groups (for the pagination UI).
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
		    SELECT dup_group_id FROM assets
		    WHERE user_id=$1 AND dup_group_id IS NOT NULL AND deleted_at IS NULL AND status='complete'
		    GROUP BY dup_group_id HAVING COUNT(*) > 1
		) g`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	dir := "DESC"
	if ascending {
		dir = "ASC"
	}
	// Pick the page of group ids first (ordered by group backup time), then pull
	// all members of just those groups.
	q := `
		WITH grp AS (
		    SELECT dup_group_id, MAX(created_at) AS ts
		    FROM assets
		    WHERE user_id=$1 AND dup_group_id IS NOT NULL AND deleted_at IS NULL AND status='complete'
		    GROUP BY dup_group_id HAVING COUNT(*) > 1
		    ORDER BY ts ` + dir + `, dup_group_id
		    LIMIT ` + itoa(limit) + ` OFFSET ` + itoa(offset) + `
		)
		SELECT a.dup_group_id, a.id, a.media_type, a.byte_size, a.width, a.height,
		       a.captured_at, a.status, a.favorite, a.storage_key, a.thumb_key
		FROM assets a
		JOIN grp ON grp.dup_group_id = a.dup_group_id
		WHERE a.user_id=$1 AND a.deleted_at IS NULL AND a.status='complete'
		ORDER BY grp.ts ` + dir + `, a.dup_group_id, a.captured_at DESC NULLS LAST, a.id DESC`

	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var groups []DupGroup
	var cur *DupGroup
	for rows.Next() {
		var gid string
		var a TimelineAsset
		if err := rows.Scan(&gid, &a.ID, &a.MediaType, &a.ByteSize, &a.Width, &a.Height,
			&a.CapturedAt, &a.Status, &a.Favorite, &a.StorageKey, &a.ThumbKey); err != nil {
			return nil, 0, err
		}
		if cur == nil || cur.GroupID != gid {
			groups = append(groups, DupGroup{GroupID: gid})
			cur = &groups[len(groups)-1]
		}
		cur.Assets = append(cur.Assets, a)
	}
	return groups, total, rows.Err()
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
