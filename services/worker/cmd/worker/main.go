package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"ashen/worker/internal/config"
	"ashen/worker/internal/job"
	"ashen/worker/internal/processor"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	s3, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	ropt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis url: %v", err)
	}
	rdb := redis.NewClient(ropt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}
	defer rdb.Close()

	proc := processor.New(pool, s3, cfg.BucketThumbnail)
	if cfg.ReplicationEnabled() {
		replica, rerr := minio.New(cfg.ReplicaEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.ReplicaAccessKey, cfg.ReplicaSecretKey, ""),
			Secure: cfg.ReplicaUseSSL,
		})
		if rerr != nil {
			log.Fatalf("replica s3: %v", rerr)
		}
		proc.WithReplica(replica, cfg.ReplicaTarget, cfg.ReplicaBucketPhotos, cfg.ReplicaBucketVideos)
		log.Printf("replication enabled -> %s (%s)", cfg.ReplicaEndpoint, cfg.ReplicaTarget)
	}

	qkey := job.QueueKey()
	rkey := job.ReplicateJobQueueKey()
	log.Printf("worker started, consuming %s + %s", qkey, rkey)

	for {
		if ctx.Err() != nil {
			log.Println("shutting down")
			return
		}
		// BRPOP blocks on both queues (up to 5s) so we can re-check ctx and exit cleanly.
		res, err := rdb.BRPop(ctx, 5*time.Second, qkey, rkey).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			log.Printf("brpop: %v", err)
			time.Sleep(time.Second)
			continue
		}
		// res = [key, value]
		jobCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

		if res[0] == rkey {
			var rj job.ReplicateJob
			if err := json.Unmarshal([]byte(res[1]), &rj); err != nil {
				log.Printf("bad replicate payload: %v", err)
				cancel()
				continue
			}
			if err := proc.Replicate(jobCtx, rj); err != nil {
				log.Printf("replicate asset=%s failed: %v", rj.AssetID, err)
			}
			cancel()
			continue
		}

		var j job.VerifyJob
		if err := json.Unmarshal([]byte(res[1]), &j); err != nil {
			log.Printf("bad job payload: %v", err)
			cancel()
			continue
		}
		if err := proc.Process(jobCtx, j); err != nil {
			log.Printf("process asset=%s failed: %v", j.AssetID, err)
			if b, mErr := json.Marshal(j); mErr == nil {
				_ = rdb.LPush(context.Background(), qkey, b).Err()
			}
		} else {
			log.Printf("processed asset=%s ok", j.AssetID)
			// Chain replication after a successful verify.
			if proc.ReplicationEnabled() {
				rj := job.ReplicateJob{AssetID: j.AssetID, MediaType: j.MediaType, Bucket: j.Bucket, StorageKey: j.StorageKey}
				if b, mErr := json.Marshal(rj); mErr == nil {
					_ = rdb.LPush(context.Background(), rkey, b).Err()
				}
			}
		}
		cancel()
	}
}
