package api

import (
	"net/http"
	"time"
)

const downloadTTL = 15 * time.Minute

type timelineItem struct {
	ID          string     `json:"id"`
	MediaType   string     `json:"media_type"`
	ByteSize    int64      `json:"byte_size"`
	Width       *int       `json:"width"`
	Height      *int       `json:"height"`
	CapturedAt  *time.Time `json:"captured_at"`
	Status      string     `json:"status"`
	ThumbURL    string     `json:"thumb_url,omitempty"`
	DownloadURL string     `json:"download_url"`
}

// GET /assets?limit=&before=RFC3339
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	limit := 60
	if v := r.URL.Query().Get("limit"); v != "" {
		if n := atoi(v); n > 0 {
			limit = n
		}
	}
	var before *time.Time
	if v := r.URL.Query().Get("before"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			before = &t
		}
	}

	assets, err := s.store.ListAssets(r.Context(), userID(r), limit, before)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}

	items := make([]timelineItem, 0, len(assets))
	for _, a := range assets {
		it := timelineItem{
			ID: a.ID, MediaType: a.MediaType, ByteSize: a.ByteSize,
			Width: a.Width, Height: a.Height, CapturedAt: a.CapturedAt, Status: a.Status,
		}
		if bucket, err := s.storage.BucketFor(a.MediaType); err == nil {
			if u, err := s.storage.PresignGet(r.Context(), bucket, a.StorageKey, downloadTTL); err == nil {
				it.DownloadURL = u
			}
		}
		if a.ThumbKey != nil && *a.ThumbKey != "" {
			if u, err := s.storage.PresignGet(r.Context(), s.storage.ThumbBucket(), *a.ThumbKey, downloadTTL); err == nil {
				it.ThumbURL = u
			}
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": items})
}

// GET /stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "stats failed")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
