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
