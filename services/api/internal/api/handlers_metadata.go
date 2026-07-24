package api

import (
	"errors"
	"net/http"

	"ashen/api/internal/store"
)

// GET /assets/{id}/metadata
func (s *Server) handleAssetMetadata(w http.ResponseWriter, r *http.Request) {
	assetID := chiURLParam(r, "id")
	uid := userID(r)

	tech, err := s.store.AssetTechnical(r.Context(), uid, assetID)
	techMissing := errors.Is(err, store.ErrNotFound)
	if err != nil && !techMissing {
		writeErr(w, http.StatusInternalServerError, "metadata lookup failed")
		return
	}

	jobs, err := s.store.ProcessingJobs(r.Context(), uid, assetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "jobs lookup failed")
		return
	}

	resp := map[string]any{
		"asset_id":          assetID,
		"processing_status": store.DeriveMetadataStatus(jobs),
	}
	if !techMissing {
		resp["technical"] = tech
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /assets/{id}/processing-jobs
func (s *Server) handleAssetJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ProcessingJobs(r.Context(), userID(r), chiURLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "jobs lookup failed")
		return
	}
	if jobs == nil {
		jobs = []store.ProcessingJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}
