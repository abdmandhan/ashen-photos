package storage

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"ashen/api/internal/config"
)

type Storage struct {
	client       *minio.Client
	bucketPhotos string
	bucketVideos string
	bucketThumb  string
}

func New(cfg config.S3Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Storage{
		client:       client,
		bucketPhotos: cfg.BucketPhotos,
		bucketVideos: cfg.BucketVideos,
		bucketThumb:  cfg.BucketThumbnail,
	}, nil
}

// ThumbBucket returns the bucket thumbnails are stored in.
func (s *Storage) ThumbBucket() string { return s.bucketThumb }

// BucketFor maps a media type to its object-storage bucket.
func (s *Storage) BucketFor(mediaType string) (string, error) {
	switch mediaType {
	case "photo":
		return s.bucketPhotos, nil
	case "video":
		return s.bucketVideos, nil
	default:
		return "", fmt.Errorf("unknown media_type %q", mediaType)
	}
}

// PresignPut returns a presigned PUT URL for a direct client upload.
func (s *Storage) PresignPut(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PresignGet returns a presigned GET URL for downloads.
func (s *Storage) PresignGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
