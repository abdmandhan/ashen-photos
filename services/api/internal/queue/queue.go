package queue

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// VerifyQueueKey is the Redis list the worker pops verify jobs from.
const VerifyQueueKey = "ashen:verify:queue"

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
}

type Queue struct {
	rdb *redis.Client
}

func New(redisURL string) (*Queue, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &Queue{rdb: redis.NewClient(opt)}, nil
}

func (q *Queue) Ping(ctx context.Context) error {
	return q.rdb.Ping(ctx).Err()
}

func (q *Queue) EnqueueVerify(ctx context.Context, job VerifyJob) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.rdb.LPush(ctx, VerifyQueueKey, b).Err()
}
