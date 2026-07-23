package queue

import (
	"context"
	"encoding/json"
	"os"

	"github.com/redis/go-redis/v9"
)

// VerifyQueueKey is the default Redis list the worker pops verify jobs from.
// Override with ASHEN_QUEUE_KEY to run an isolated pipeline (e.g. for testing
// alongside a production worker on the same Redis).
const VerifyQueueKey = "ashen:verify:queue"

func queueKey() string {
	if k := os.Getenv("ASHEN_QUEUE_KEY"); k != "" {
		return k
	}
	return VerifyQueueKey
}

// ReplicateQueueKey is the default list for replication jobs.
const ReplicateQueueKey = "ashen:replicate:queue"

func replicateKey() string {
	if k := os.Getenv("ASHEN_QUEUE_KEY"); k != "" {
		return k + ":replicate"
	}
	return ReplicateQueueKey
}

// ReplicateJob mirrors the worker's job.ReplicateJob (same JSON shape).
type ReplicateJob struct {
	AssetID    string `json:"asset_id"`
	MediaType  string `json:"media_type"`
	Bucket     string `json:"bucket"`
	StorageKey string `json:"storage_key"`
}

// VerifyJob asks the worker to download an uploaded object, recompute its
// SHA-256, and (on match) generate a thumbnail + extract EXIF.
type VerifyJob struct {
	UploadID   string `json:"upload_id"`
	AssetID    string `json:"asset_id"`
	UserID     string `json:"user_id"`
	Bucket     string `json:"bucket"`
	StorageKey string `json:"storage_key"`
	SHA256     string `json:"sha256"`
	MediaType  string `json:"media_type"`
	ThumbKey   string `json:"thumb_key,omitempty"` // client-uploaded thumbnail (HEIC/video)
}

type Queue struct {
	rdb    *redis.Client
	key    string
	repKey string
}

func New(redisURL string) (*Queue, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &Queue{rdb: redis.NewClient(opt), key: queueKey(), repKey: replicateKey()}, nil
}

func (q *Queue) EnqueueReplicate(ctx context.Context, job ReplicateJob) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.rdb.LPush(ctx, q.repKey, b).Err()
}

func (q *Queue) Ping(ctx context.Context) error {
	return q.rdb.Ping(ctx).Err()
}

func (q *Queue) EnqueueVerify(ctx context.Context, job VerifyJob) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.rdb.LPush(ctx, q.key, b).Err()
}
