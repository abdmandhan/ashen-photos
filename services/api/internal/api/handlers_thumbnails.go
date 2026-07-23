package api

import (
	"errors"
	"net/http"

	"ashen/api/internal/store"
)

// GET /thumbnails/missing — sha256s of the user's assets lacking a thumbnail.
func (s *Server) handleMissingThumbs(w http.ResponseWriter, r *http.Request) {
	shas, err := s.store.MissingThumbShas(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	if shas == nil {
		shas = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"shas": shas})
}

// POST /thumbnails/presign  {sha256}  → presigned PUT to the thumb slot.
func (s *Server) handlePresignThumb(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SHA256 string `json:"sha256"`
	}
	if err := decode(r, &in); err != nil || in.SHA256 == "" {
		writeErr(w, http.StatusBadRequest, "sha256 required")
		return
	}
	thumbKey := userID(r) + "/" + in.SHA256 + ".jpg"
	url, err := s.storage.PresignPut(r.Context(), s.storage.ThumbBucket(), thumbKey, presignTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "presign failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thumb_key": thumbKey, "thumb_put_url": url})
}

// POST /thumbnails/commit  {sha256}  → set thumb_key after the client PUT the thumbnail.
func (s *Server) handleCommitThumb(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SHA256 string `json:"sha256"`
	}
	if err := decode(r, &in); err != nil || in.SHA256 == "" {
		writeErr(w, http.StatusBadRequest, "sha256 required")
		return
	}
	thumbKey := userID(r) + "/" + in.SHA256 + ".jpg"
	err := s.store.SetThumbBySha(r.Context(), userID(r), in.SHA256, thumbKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
