package main

import (
	"context"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"ashen/metadata-worker/internal/config"
	"ashen/metadata-worker/internal/extract"
	"ashen/metadata-worker/internal/jobs"
	"ashen/metadata-worker/internal/processor"
	"ashen/metadata-worker/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
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

	redisOpt, err := cfg.RedisOpt()
	if err != nil {
		log.Fatalf("redis: %v", err)
	}

	client := asynq.NewClient(redisOpt)
	defer client.Close()

	proc := processor.New(store.New(pool), extract.New(s3), client)

	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TypeExtract, proc.HandleExtract)
	mux.HandleFunc(jobs.TypeNormalize, proc.HandleNormalize)
	mux.HandleFunc(jobs.TypeIndex, proc.HandleIndex)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Concurrency,
		Queues:      map[string]int{"metadata": 10},
		// Retry ladder: 1m, 5m, 30m, 2h, 12h (req Retry Strategy).
		RetryDelayFunc: func(n int, _ error, _ *asynq.Task) time.Duration {
			ladder := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}
			if n-1 < len(ladder) {
				return ladder[n-1]
			}
			return 12 * time.Hour
		},
	})

	log.Printf("metadata worker started (version %s), consuming queue 'metadata'", jobs.WorkerVersion)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("asynq server: %v", err)
	}
}
