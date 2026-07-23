package api

import (
	"net/http"
	"time"

	"ashen/api/internal/store"
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
	Favorite    bool       `json:"favorite"`
	ThumbURL    string     `json:"thumb_url,omitempty"`
	DownloadURL string     `json:"download_url"`
}

// GET /assets?from=&to=&media_type=&device_id=&favorite=&album_id=&limit=&before=RFC3339
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AssetFilter{
		MediaType: q.Get("media_type"),
		AlbumID:   q.Get("album_id"),
		DeviceID:  q.Get("device_id"),
		Limit:     atoi(q.Get("limit")),
		From:      parseTime(q.Get("from")),
		To:        parseTime(q.Get("to")),
		Before:    parseTime(q.Get("before")),
	}
	if v := q.Get("favorite"); v == "true" || v == "false" {
		b := v == "true"
		f.Favorite = &b
	}

	assets, err := s.store.ListAssetsFiltered(r.Context(), userID(r), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": s.presignTimeline(r, assets)})
}

// GET /search/facets — counts for the filter UI.
func (s *Server) handleFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := s.store.FacetCounts(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "facets failed")
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func parseTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	return nil
}

// presignTimeline turns store assets into API items with presigned thumb + download URLs.
func (s *Server) presignTimeline(r *http.Request, assets []store.TimelineAsset) []timelineItem {
	items := make([]timelineItem, 0, len(assets))
	for _, a := range assets {
		it := timelineItem{
			ID: a.ID, MediaType: a.MediaType, ByteSize: a.ByteSize,
			Width: a.Width, Height: a.Height, CapturedAt: a.CapturedAt, Status: a.Status,
			Favorite: a.Favorite,
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
	return items
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
