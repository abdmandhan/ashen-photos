package config

import (
	"fmt"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	DatabaseURL  string
	RedisURL     string
	S3Endpoint   string
	S3AccessKey  string
	S3SecretKey  string
	S3UseSSL     bool
	Concurrency  int
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		S3Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		S3AccessKey: os.Getenv("MINIO_ROOT_USER"),
		S3SecretKey: os.Getenv("MINIO_ROOT_PASSWORD"),
		S3UseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
		Concurrency: 10,
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, fmt.Errorf("REDIS_URL is required")
	}
	return c, nil
}

// RedisOpt converts REDIS_URL into an Asynq redis connection option.
func (c Config) RedisOpt() (asynq.RedisClientOpt, error) {
	opt, err := redis.ParseURL(c.RedisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, err
	}
	return asynq.RedisClientOpt{Addr: opt.Addr, Password: opt.Password, DB: opt.DB}, nil
}
