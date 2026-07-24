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

// GET /duplicates?limit=&offset=&sort=oldest — paginated near-duplicate groups.
func (s *Server) handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := atoi(q.Get("limit"))
	if limit == 0 {
		limit = 20
	}
	offset := atoi(q.Get("offset"))
	ascending := q.Get("sort") == "oldest"

	groups, total, err := s.store.DuplicateGroups(r.Context(), userID(r), limit, offset, ascending)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"groups": out, "total": total, "limit": limit, "offset": offset,
	})
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

// POST /duplicates/{group_id}/keep  body {"keep_asset_id":"..."}
// Keeps one asset and soft-deletes the rest of the group.
func (s *Server) handleKeepDuplicate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		KeepAssetID string `json:"keep_asset_id"`
	}
	if err := decode(r, &in); err != nil || in.KeepAssetID == "" {
		writeErr(w, http.StatusBadRequest, "keep_asset_id required")
		return
	}
	deleted, err := s.store.KeepDuplicate(r.Context(), userID(r), chiURLParam(r, "group_id"), in.KeepAssetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not in group")
			return
		}
		writeErr(w, http.StatusInternalServerError, "keep failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}
