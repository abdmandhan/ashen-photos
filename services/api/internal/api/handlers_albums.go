package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ashen/api/internal/store"
)

// --- Albums ---

type albumResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	CoverAssetID *string   `json:"cover_asset_id"`
	AssetCount   int       `json:"asset_count"`
	CoverURL     string    `json:"cover_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Server) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	a, err := s.store.CreateAlbum(r.Context(), userID(r), in.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create album failed")
		return
	}
	writeJSON(w, http.StatusCreated, albumResponse{
		ID: a.ID, Name: a.Name, CoverAssetID: a.CoverAssetID,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	})
}

func (s *Server) handleListAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := s.store.ListAlbums(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list albums failed")
		return
	}
	out := make([]albumResponse, 0, len(albums))
	for _, a := range albums {
		resp := albumResponse{
			ID: a.ID, Name: a.Name, CoverAssetID: a.CoverAssetID,
			AssetCount: a.AssetCount, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		}
		if a.CoverThumb != nil && *a.CoverThumb != "" {
			if u, err := s.storage.PresignGet(r.Context(), s.storage.ThumbBucket(), *a.CoverThumb, downloadTTL); err == nil {
				resp.CoverURL = u
			}
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": out})
}

func (s *Server) handleUpdateAlbum(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         *string `json:"name"`
		CoverAssetID *string `json:"cover_asset_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := s.store.UpdateAlbum(r.Context(), userID(r), chiURLParam(r, "id"), in.Name, in.CoverAssetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteAlbum(r.Context(), userID(r), chiURLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Album membership ---

func (s *Server) handleAddAlbumAsset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AssetID string `json:"asset_id"`
	}
	if err := decode(r, &in); err != nil || in.AssetID == "" {
		writeErr(w, http.StatusBadRequest, "asset_id required")
		return
	}
	err := s.store.AddAssetToAlbum(r.Context(), userID(r), chiURLParam(r, "id"), in.AssetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "album or asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "add failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *Server) handleRemoveAlbumAsset(w http.ResponseWriter, r *http.Request) {
	err := s.store.RemoveAssetFromAlbum(r.Context(), userID(r), chiURLParam(r, "id"), chiURLParam(r, "assetId"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "remove failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handleListAlbumAssets(w http.ResponseWriter, r *http.Request) {
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
	assets, err := s.store.ListAlbumAssets(r.Context(), userID(r), chiURLParam(r, "id"), limit, before)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": s.presignTimeline(r, assets)})
}

// --- Favorites ---

func (s *Server) handleSetFavorite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Favorite bool `json:"favorite"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := s.store.SetFavorite(r.Context(), userID(r), chiURLParam(r, "id"), in.Favorite)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "asset not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": in.Favorite})
}
