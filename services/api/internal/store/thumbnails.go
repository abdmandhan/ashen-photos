package store

import "context"

// MissingThumbShas returns sha256s of the user's completed, non-deleted assets
// that have no thumbnail yet (HEIC/video backed up before client thumbnails).
func (s *Store) MissingThumbShas(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sha256 FROM assets
		 WHERE user_id=$1 AND status='complete' AND deleted_at IS NULL
		   AND (thumb_key IS NULL OR thumb_key='')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sha string
		if err := rows.Scan(&sha); err != nil {
			return nil, err
		}
		out = append(out, sha)
	}
	return out, rows.Err()
}

// SetThumbBySha sets an asset's thumb_key by (user, sha256). Returns ErrNotFound
// if no matching asset.
func (s *Store) SetThumbBySha(ctx context.Context, userID, sha, thumbKey string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE assets SET thumb_key=$3 WHERE user_id=$1 AND sha256=$2`, userID, sha, thumbKey)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
