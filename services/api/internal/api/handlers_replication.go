package api

import (
	"net/http"

	"ashen/api/internal/queue"
)

// GET /replication/status
func (s *Server) handleReplicationStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.ReplicationStatus(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "status failed")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// POST /replication/redrive — enqueue replication for unreplicated/failed assets.
func (s *Server) handleReplicationRedrive(w http.ResponseWriter, r *http.Request) {
	assets, err := s.store.AssetsNeedingReplication(r.Context(), userID(r), 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "redrive query failed")
		return
	}
	queued := 0
	for _, a := range assets {
		bucket, berr := s.storage.BucketFor(a.MediaType)
		if berr != nil {
			continue
		}
		if err := s.queue.EnqueueReplicate(r.Context(), queue.ReplicateJob{
			AssetID:    a.AssetID,
			MediaType:  a.MediaType,
			Bucket:     bucket,
			StorageKey: a.StorageKey,
		}); err != nil {
			continue
		}
		queued++
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": queued})
}
