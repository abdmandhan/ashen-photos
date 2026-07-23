package api

import (
	"errors"
	"log"
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
	Items    []checkItem `json:"items"`
	DeviceID *string     `json:"device_id"`
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
		// Reconcile: this device also holds an already-backed-up asset. Record it
		// without re-uploading bytes (multi-device dedup).
		if ok && in.DeviceID != nil && *in.DeviceID != "" {
			s.store.RecordAssetDevice(r.Context(), id, *in.DeviceID)
		}
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
	UploadID    string `json:"upload_id"`
	AssetID     string `json:"asset_id"`
	StorageKey  string `json:"storage_key"`
	PutURL      string `json:"put_url"`
	ThumbKey    string `json:"thumb_key"`
	ThumbPutURL string `json:"thumb_put_url"`
	ExpiresIn   int    `json:"expires_in"`
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
	if in.DeviceID != nil && *in.DeviceID != "" {
		s.store.RecordAssetDevice(r.Context(), asset.ID, *in.DeviceID)
	}

	putURL, err := s.storage.PresignPut(r.Context(), bucket, asset.StorageKey, presignTTL)
	if err != nil {
		log.Printf("presign PUT failed bucket=%s key=%s: %v", bucket, asset.StorageKey, err)
		writeErr(w, http.StatusInternalServerError, "presign failed")
		return
	}

	// Client-provided thumbnail slot: the app can PUT a JPEG thumbnail here (works
	// for HEIC/video that the worker can't decode). Key mirrors the worker's scheme.
	thumbKey := uid + "/" + in.SHA256 + ".jpg"
	thumbPutURL, err := s.storage.PresignPut(r.Context(), s.storage.ThumbBucket(), thumbKey, presignTTL)
	if err != nil {
		log.Printf("presign thumb PUT failed key=%s: %v", thumbKey, err)
		// Non-fatal: original upload still proceeds; worker falls back to generating.
		thumbKey, thumbPutURL = "", ""
	}

	writeJSON(w, http.StatusCreated, createUploadResponse{
		UploadID:    upload.ID,
		AssetID:     asset.ID,
		StorageKey:  asset.StorageKey,
		PutURL:      putURL,
		ThumbKey:    thumbKey,
		ThumbPutURL: thumbPutURL,
		ExpiresIn:   int(presignTTL.Seconds()),
	})
}

// --- POST /uploads/{id}/complete ---

func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chiURLParam(r, "id")

	// Optional body: {"thumb": true} means the client uploaded a thumbnail to the
	// presigned thumb slot (used for HEIC/video the worker can't decode).
	var in struct {
		Thumb bool `json:"thumb"`
	}
	_ = decode(r, &in) // body optional; ignore decode errors

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
	thumbKey := ""
	if in.Thumb {
		thumbKey = detail.UserID + "/" + detail.SHA256 + ".jpg"
	}
	if err := s.queue.EnqueueVerify(r.Context(), queue.VerifyJob{
		UploadID:   detail.UploadID,
		AssetID:    detail.AssetID,
		UserID:     detail.UserID,
		Bucket:     bucket,
		StorageKey: detail.StorageKey,
		SHA256:     detail.SHA256,
		MediaType:  detail.MediaType,
		ThumbKey:   thumbKey,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "enqueue failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "asset_id": detail.AssetID})
}
