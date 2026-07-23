package config

import (
	"fmt"
	"os"
)

type S3Config struct {
	Endpoint        string
	AccessKey       string
	SecretKey       string
	UseSSL          bool
	Region          string
	BucketPhotos    string
	BucketVideos    string
	BucketThumbnail string
}

type Config struct {
	Addr        string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	S3          S3Config
}

// Load reads config from the environment. DATABASE_URL and JWT_SECRET are required.
func Load() (Config, error) {
	c := Config{
		Addr:        env("API_ADDR", ":8082"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		S3: S3Config{
			Endpoint:        os.Getenv("MINIO_ENDPOINT"),
			AccessKey:       os.Getenv("MINIO_ROOT_USER"),
			SecretKey:       os.Getenv("MINIO_ROOT_PASSWORD"),
			UseSSL:          os.Getenv("MINIO_USE_SSL") == "true",
			Region:          env("MINIO_REGION", "us-east-1"),
			BucketPhotos:    env("S3_BUCKET_PHOTOS", "ashen-photos"),
			BucketVideos:    env("S3_BUCKET_VIDEOS", "ashen-videos"),
			BucketThumbnail: env("S3_BUCKET_THUMBNAILS", "ashen-thumbnails"),
		},
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return c, fmt.Errorf("JWT_SECRET is required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
