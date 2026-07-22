package api

import (
	"errors"
	"net/http"
	"time"

	"ashen/api/internal/queue"
	"ashen/api/internal/store"
)

const presignTTL = 15 * time.Minute

// --- POST /uploads/check ---

type checkItem struct {
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
}
type checkRequest struct {
	Items []checkItem `json:"items"`
}
type checkResult struct {
	SHA256  string `json:"sha256"`
	Exists  bool   `json:"exists"`
	AssetID string `json:"asset_id,omitempty"`
}

func (s *Server) handleUploadCheck(w http.ResponseWriter, r *http.Request) {
	var in checkRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(in.Items) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"results": []checkResult{}})
		return
	}
	hashes := make([]string, len(in.Items))
	for i, it := range in.Items {
		hashes[i] = it.SHA256
	}
	existing, err := s.store.ExistingHashes(r.Context(), userID(r), hashes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "check failed")
		return
	}
	results := make([]checkResult, len(in.Items))
	for i, it := range in.Items {
		id, ok := existing[it.SHA256]
		results[i] = checkResult{SHA256: it.SHA256, Exists: ok, AssetID: id}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// --- POST /uploads ---

type createUploadRequest struct {
	SHA256     string     `json:"sha256"`
	MediaType  string     `json:"media_type"`
	ByteSize   int64      `json:"byte_size"`
	CapturedAt       *time.Time `json:"captured_at"`
	DeviceID         *string    `json:"device_id"`
	Ext              string     `json:"ext"`
	LivePhotoGroupID *string    `json:"live_photo_group_id"`
}
type createUploadResponse struct {
	UploadID   string `json:"upload_id"`
	AssetID    string `json:"asset_id"`
	StorageKey string `json:"storage_key"`
	PutURL     string `json:"put_url"`
	ExpiresIn  int    `json:"expires_in"`
}

func (s *Server) handleCreateUpload(w http.ResponseWriter, r *http.Request) {
	var in createUploadRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if in.SHA256 == "" || in.ByteSize <= 0 {
		writeErr(w, http.StatusBadRequest, "sha256 and byte_size required")
		return
	}
	if in.MediaType != "photo" && in.MediaType != "video" {
		writeErr(w, http.StatusBadRequest, "media_type must be photo or video")
		return
	}
	bucket, err := s.storage.BucketFor(in.MediaType)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	uid := userID(r)
	key := uid + "/" + in.SHA256
	if in.Ext != "" {
		key += "." + in.Ext
	}

	asset, err := s.store.CreateAsset(r.Context(), store.Asset{
		UserID:     uid,
		SHA256:     in.SHA256,
		MediaType:  in.MediaType,
		ByteSize:         in.ByteSize,
		CapturedAt:       in.CapturedAt,
		StorageKey:       key,
		LivePhotoGroupID: in.LivePhotoGroupID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create asset failed")
		return
	}

	upload, err := s.store.CreateUpload(r.Context(), asset.ID, in.DeviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create upload failed")
		return
	}

	putURL, err := s.storage.PresignPut(r.Context(), bucket, asset.StorageKey, presignTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "presign failed")
		return
	}

	writeJSON(w, http.StatusCreated, createUploadResponse{
		UploadID:   upload.ID,
		AssetID:    asset.ID,
		StorageKey: asset.StorageKey,
		PutURL:     putURL,
		ExpiresIn:  int(presignTTL.Seconds()),
	})
}

// --- POST /uploads/{id}/complete ---

func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chiURLParam(r, "id")
	detail, err := s.store.UploadDetail(r.Context(), userID(r), uploadID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "upload not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if err := s.store.MarkUploaded(r.Context(), detail.UploadID, detail.AssetID); err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	bucket, err := s.storage.BucketFor(detail.MediaType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.queue.EnqueueVerify(r.Context(), queue.VerifyJob{
		UploadID:   detail.UploadID,
		AssetID:    detail.AssetID,
		UserID:     detail.UserID,
		Bucket:     bucket,
		StorageKey: detail.StorageKey,
		SHA256:     detail.SHA256,
		MediaType:  detail.MediaType,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "enqueue failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "asset_id": detail.AssetID})
}
