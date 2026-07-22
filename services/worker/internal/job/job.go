package job

// VerifyQueueKey must match the API's queue.VerifyQueueKey.
const VerifyQueueKey = "ashen:verify:queue"

// VerifyJob mirrors the API's queue.VerifyJob (same JSON shape).
type VerifyJob struct {
	UploadID   string `json:"upload_id"`
	AssetID    string `json:"asset_id"`
	UserID     string `json:"user_id"`
	Bucket     string `json:"bucket"`
	StorageKey string `json:"storage_key"`
	SHA256     string `json:"sha256"`
	MediaType  string `json:"media_type"`
}
