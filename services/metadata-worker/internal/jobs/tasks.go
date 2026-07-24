package jobs

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// WorkerVersion bumps when extraction/normalization logic changes (for reprocessing).
const WorkerVersion = "1"

// Task types (Asynq). Must match the enqueuer in the verify worker.
const (
	TypeExtract   = "metadata:extract"
	TypeNormalize = "metadata:normalize"
	TypeIndex     = "metadata:index"
)

// ExtractPayload is enqueued when an asset reaches `complete`.
type ExtractPayload struct {
	AssetID    string `json:"asset_id"`
	UserID     string `json:"user_id"`
	Bucket     string `json:"bucket"`
	StorageKey string `json:"storage_key"`
	MediaType  string `json:"media_type"`
	SHA256     string `json:"sha256"`
}

// AssetPayload carries just the asset id for downstream tasks.
type AssetPayload struct {
	AssetID string `json:"asset_id"`
}

// NewExtractTask builds a deduplicated extract task (one per asset+version).
func NewExtractTask(p ExtractPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	// TaskID makes enqueue idempotent (BR-006): a duplicate backup event for the
	// same asset+version is rejected while the task is still retained.
	id := "extract:" + p.AssetID + ":" + WorkerVersion
	return asynq.NewTask(TypeExtract, b, asynq.TaskID(id), asynq.Queue("metadata")), nil
}

func NewAssetTask(taskType, assetID string) (*asynq.Task, error) {
	b, err := json.Marshal(AssetPayload{AssetID: assetID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskType, b, asynq.Queue("metadata")), nil
}
