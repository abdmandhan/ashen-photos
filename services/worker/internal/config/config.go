package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL     string
	RedisURL        string
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string
	S3UseSSL        bool
	BucketThumbnail string

	// Secondary replication target (optional). Empty endpoint = replication off.
	ReplicaEndpoint     string
	ReplicaAccessKey    string
	ReplicaSecretKey    string
	ReplicaUseSSL       bool
	ReplicaTarget       string // label stored in asset_replicas.target
	ReplicaBucketPhotos string
	ReplicaBucketVideos string
}

func (c Config) ReplicationEnabled() bool { return c.ReplicaEndpoint != "" }

// ReplicaBucketFor maps a media type to its replica bucket.
func (c Config) ReplicaBucketFor(mediaType string) string {
	if mediaType == "video" {
		return c.ReplicaBucketVideos
	}
	return c.ReplicaBucketPhotos
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisURL:        os.Getenv("REDIS_URL"),
		S3Endpoint:      os.Getenv("MINIO_ENDPOINT"),
		S3AccessKey:     os.Getenv("MINIO_ROOT_USER"),
		S3SecretKey:     os.Getenv("MINIO_ROOT_PASSWORD"),
		S3UseSSL:        os.Getenv("MINIO_USE_SSL") == "true",
		BucketThumbnail: env("S3_BUCKET_THUMBNAILS", "ashen-thumbnails"),

		ReplicaEndpoint:     os.Getenv("REPLICA_ENDPOINT"),
		ReplicaAccessKey:    os.Getenv("REPLICA_ACCESS_KEY"),
		ReplicaSecretKey:    os.Getenv("REPLICA_SECRET_KEY"),
		ReplicaUseSSL:       os.Getenv("REPLICA_USE_SSL") == "true",
		ReplicaTarget:       env("REPLICA_TARGET", "replica"),
		ReplicaBucketPhotos: env("REPLICA_BUCKET_PHOTOS", "ashen-photos"),
		ReplicaBucketVideos: env("REPLICA_BUCKET_VIDEOS", "ashen-videos"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, fmt.Errorf("REDIS_URL is required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
