package miniorepo

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorageRepository struct {
	client         *minio.Client
	presignClient  *minio.Client
	bucket         string
	publicEndpoint string
	bucketLookup   minio.BucketLookupType
}

func New() (*MinIOStorageRepository, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	bucket := os.Getenv("MINIO_BUCKET")
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"
	publicEndpoint := os.Getenv("MINIO_PUBLIC_ENDPOINT")

	region := os.Getenv("MINIO_REGION")
	if region == "" {
		region = "us-east-1"
	}
	// ローカル MinIO は path-style、AWS S3 は DNS-style（virtual-hosted）
	bucketLookup := minio.BucketLookupPath
	if os.Getenv("MINIO_BUCKET_LOOKUP") == "dns" {
		bucketLookup = minio.BucketLookupDNS
	}

	creds := credentials.NewStaticV4(accessKey, secretKey, "")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        creds,
		Secure:       useSSL,
		Region:       region,
		BucketLookup: bucketLookup,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	presignClient := client
	if publicEndpoint != "" {
		pub, parseErr := url.Parse(publicEndpoint)
		if parseErr == nil {
			pc, clientErr := minio.New(pub.Host, &minio.Options{
				Creds:        creds,
				Secure:       pub.Scheme == "https",
				Region:       region,
				BucketLookup: bucketLookup,
			})
			if clientErr == nil {
				presignClient = pc
			}
		}
	}

	return &MinIOStorageRepository{
		client:         client,
		presignClient:  presignClient,
		bucket:         bucket,
		publicEndpoint: publicEndpoint,
		bucketLookup:   bucketLookup,
	}, nil
}

func (r *MinIOStorageRepository) PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration, _ int64) (string, error) {
	// presignClient は公開エンドポイントで初期化されているため、
	// 生成される URL のホストはすでに localhost:9000 になっている。
	// maxBytes は PUT presigned URL では強制できないため無視する。
	// サーバー側での厳密なサイズ制限が必要な場合は presigned POST policy
	// またはバケットポリシーへの移行を検討すること。
	u, err := r.presignClient.PresignedPutObject(ctx, r.bucket, objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url: %w", err)
	}
	return u.String(), nil
}

func (r *MinIOStorageRepository) PublicURL(objectKey string) string {
	if r.bucketLookup == minio.BucketLookupDNS {
		u, err := url.Parse(r.publicEndpoint)
		if err == nil {
			return fmt.Sprintf("%s://%s.%s/%s", u.Scheme, r.bucket, u.Host, objectKey)
		}
	}
	return fmt.Sprintf("%s/%s/%s", r.publicEndpoint, r.bucket, objectKey)
}

func (r *MinIOStorageRepository) DeleteObject(ctx context.Context, objectKey string) error {
	return r.client.RemoveObject(ctx, r.bucket, objectKey, minio.RemoveObjectOptions{})
}
