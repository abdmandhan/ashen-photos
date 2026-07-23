package store

import "context"

type ReplicationStatus struct {
	Replicated  int64 `json:"replicated"`
	Failed      int64 `json:"failed"`
	Pending     int64 `json:"pending"`
	Unreplicated int64 `json:"unreplicated"` // complete assets with no replica row yet
}

// ReplicationStatus summarizes replication for the user across all targets.
func (s *Store) ReplicationStatus(ctx context.Context, userID string) (ReplicationStatus, error) {
	var st ReplicationStatus
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE ar.status='replicated'),
		  COUNT(*) FILTER (WHERE ar.status='failed'),
		  COUNT(*) FILTER (WHERE ar.status='pending')
		FROM asset_replicas ar
		JOIN assets a ON a.id = ar.asset_id
		WHERE a.user_id=$1`, userID,
	).Scan(&st.Replicated, &st.Failed, &st.Pending)
	if err != nil {
		return st, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM assets a
		WHERE a.user_id=$1 AND a.status='complete' AND a.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM asset_replicas ar
		                  WHERE ar.asset_id=a.id AND ar.status='replicated')`, userID,
	).Scan(&st.Unreplicated)
	return st, err
}

// AssetToReplicate is the minimum needed to enqueue a replication job.
type AssetToReplicate struct {
	AssetID    string
	MediaType  string
	StorageKey string
}

// AssetsNeedingReplication returns completed assets that aren't yet replicated
// (missing or failed), for redrive.
func (s *Store) AssetsNeedingReplication(ctx context.Context, userID string, limit int) ([]AssetToReplicate, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.media_type, a.storage_key
		FROM assets a
		WHERE a.user_id=$1 AND a.status='complete' AND a.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM asset_replicas ar
		                  WHERE ar.asset_id=a.id AND ar.status='replicated')
		ORDER BY a.created_at DESC
		LIMIT `+itoa(limit), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetToReplicate
	for rows.Next() {
		var a AssetToReplicate
		if err := rows.Scan(&a.AssetID, &a.MediaType, &a.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
