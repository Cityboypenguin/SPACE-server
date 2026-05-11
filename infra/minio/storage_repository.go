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
	client         *minio.Client // 内部 API 呼び出し用 (minio:9000)
	presignClient  *minio.Client // Presigned URL 生成用 (localhost:9000)
	bucket         string
	publicEndpoint string
}

func New() (*MinIOStorageRepository, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	bucket := os.Getenv("MINIO_BUCKET")
	useSSL := os.Getenv("MINIO_USE_SSL") == "true"
	publicEndpoint := os.Getenv("MINIO_PUBLIC_ENDPOINT")

	creds := credentials.NewStaticV4(accessKey, secretKey, "")

	// Docker 内部ネットワーク用クライアント（DeleteObject などに使用）
	client, err := minio.New(endpoint, &minio.Options{
		Creds:        creds,
		Secure:       useSSL,
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// Presigned URL 生成専用クライアント。
	// 公開エンドポイント (localhost:9000) で署名することで、
	// ブラウザが同じホストに PUT したときに署名が一致する。
	// Region を明示することで GetBucketLocation のネットワーク呼び出しを防ぐ。
	presignClient := client
	if publicEndpoint != "" {
		pub, parseErr := url.Parse(publicEndpoint)
		if parseErr == nil {
			pc, clientErr := minio.New(pub.Host, &minio.Options{
				Creds:        creds,
				Secure:       pub.Scheme == "https",
				Region:       "us-east-1",
				BucketLookup: minio.BucketLookupPath,
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
	return fmt.Sprintf("%s/%s/%s", r.publicEndpoint, r.bucket, objectKey)
}

func (r *MinIOStorageRepository) DeleteObject(ctx context.Context, objectKey string) error {
	return r.client.RemoveObject(ctx, r.bucket, objectKey, minio.RemoveObjectOptions{})
}
