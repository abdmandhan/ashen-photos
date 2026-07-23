package api

import (
	"errors"
	"net/http"

	"ashen/api/internal/store"
)

type dupGroupResponse struct {
	GroupID string         `json:"group_id"`
	Assets  []timelineItem `json:"assets"`
}

// GET /duplicates — near-duplicate groups for review.
func (s *Server) handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.DuplicateGroups(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list duplicates failed")
		return
	}
	out := make([]dupGroupResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, dupGroupResponse{
			GroupID: g.GroupID,
			Assets:  s.presignTimeline(r, g.Assets),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// POST /assets/{id}/resolve-duplicate  body {"action":"delete"|"keep"}
func (s *Server) handleResolveDuplicate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if in.Action != "delete" && in.Action != "keep" {
		writeErr(w, http.StatusBadRequest, "action must be delete or keep")
		return
	}
	err := s.store.ResolveDuplicate(r.Context(), userID(r), chiURLParam(r, "id"), in.Action)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "resolve failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": in.Action})
}
