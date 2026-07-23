package job

import "os"

// VerifyQueueKey must match the API's queue.VerifyQueueKey.
const VerifyQueueKey = "ashen:verify:queue"

// QueueKey returns the queue to consume, honoring ASHEN_QUEUE_KEY (matches the API).
func QueueKey() string {
	if k := os.Getenv("ASHEN_QUEUE_KEY"); k != "" {
		return k
	}
	return VerifyQueueKey
}

// VerifyJob mirrors the API's queue.VerifyJob (same JSON shape).
type VerifyJob struct {
	UploadID   string `json:"upload_id"`
	AssetID    string `json:"asset_id"`
	UserID     string `json:"user_id"`
	Bucket     string `json:"bucket"`
	StorageKey string `json:"storage_key"`
	SHA256     string `json:"sha256"`
	MediaType  string `json:"media_type"`
	ThumbKey   string `json:"thumb_key,omitempty"` // client-uploaded thumbnail
}

// ReplicateQueueKey is the list for replication jobs.
const ReplicateQueueKey = "ashen:replicate:queue"

// ReplicateJobQueueKey honors ASHEN_QUEUE_KEY (suffixed) to stay isolated with verify.
func ReplicateJobQueueKey() string {
	if k := os.Getenv("ASHEN_QUEUE_KEY"); k != "" {
		return k + ":replicate"
	}
	return ReplicateQueueKey
}

// ReplicateJob mirrors the API's queue.ReplicateJob.
type ReplicateJob struct {
	AssetID    string `json:"asset_id"`
	MediaType  string `json:"media_type"`
	Bucket     string `json:"bucket"` // source bucket
	StorageKey string `json:"storage_key"`
}
