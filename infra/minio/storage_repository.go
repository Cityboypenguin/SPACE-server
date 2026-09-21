package miniorepo

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorageRepository struct {
	client         *minio.Client
	presignClient  *minio.Client
	bucket         string
	privateBucket  string
	publicEndpoint string
	bucketLookup   minio.BucketLookupType
}

func New() (*MinIOStorageRepository, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	bucket := os.Getenv("MINIO_BUCKET")
	privateBucket := os.Getenv("MINIO_PRIVATE_BUCKET")
	if privateBucket == "" {
		privateBucket = bucket + "-private"
	}
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
		privateBucket:  privateBucket,
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

// StatObject は保存済みオブジェクトの実寸と Content-Type を返す。
// 署名付き PUT では上限を縛れないので、受け入れ時にここで実物を測る
// （repository.StorageRepository のコメント参照）。
func (r *MinIOStorageRepository) StatObject(ctx context.Context, objectKey string) (repository.ObjectInfo, error) {
	info, err := r.client.StatObject(ctx, r.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return repository.ObjectInfo{}, repository.ErrObjectNotFound
		}
		return repository.ObjectInfo{}, err
	}
	return repository.ObjectInfo{Size: info.Size, ContentType: info.ContentType, ETag: info.ETag}, nil
}

// CopyObject は同じバケット内でオブジェクトを写す。中身はこのプロセスを通らない
// （S3 の COPY は保管側で完結する）。
//
// MatchETag で「測ったときの中身と同じなら」という条件を付ける。S3 は
// x-amz-copy-source-if-match が一致しなければ 412 を返して何も書かない。
//
// 宛先側の条件（If-None-Match: *）は付けない。MinIO はこのヘッダーを黙って
// 無視し、既に在る宛先をそのまま上書きする（実機で確認済み）。付ければ守られて
// いるように見えて、実際には素通しになる。宛先が一度きりしか書かれないことは
// 呼び出し側が宛先キーを毎回引き直すことで保証している
// （internal/upload.acceptedKeyFor）。
func (r *MinIOStorageRepository) CopyObject(ctx context.Context, srcKey, dstKey, srcETag string) error {
	if srcETag == "" {
		// 条件を付けられないなら写さない。無条件に写すと、検査してから写すまでの
		// 隙間に差し替えられたものをそのまま公開することになる。
		return fmt.Errorf("refusing to copy %q without a source ETag", srcKey)
	}
	_, err := r.client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: r.bucket, Object: dstKey},
		minio.CopySrcOptions{Bucket: r.bucket, Object: srcKey, MatchETag: srcETag},
	)
	if err != nil {
		switch minio.ToErrorResponse(err).Code {
		case "NoSuchKey":
			return repository.ErrObjectNotFound
		case "PreconditionFailed":
			return repository.ErrObjectChanged
		}
		return err
	}
	return nil
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

func (r *MinIOStorageRepository) PutPrivateObject(ctx context.Context, objectKey, contentType string, body io.Reader, size int64) error {
	_, err := r.client.PutObject(ctx, r.privateBucket, objectKey, body, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (r *MinIOStorageRepository) OpenPrivateObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	object, err := r.client.GetObject(ctx, r.privateBucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject can defer a missing-object error until the first read.
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		return nil, err
	}
	return object, nil
}

func (r *MinIOStorageRepository) DeletePrivateObject(ctx context.Context, objectKey string) error {
	return r.client.RemoveObject(ctx, r.privateBucket, objectKey, minio.RemoveObjectOptions{})
}

func (r *MinIOStorageRepository) ListPrivateObjects(ctx context.Context, prefix string) ([]repository.PrivateObject, error) {
	var objects []repository.PrivateObject
	for item := range r.client.ListObjects(ctx, r.privateBucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if item.Err != nil {
			return nil, item.Err
		}
		objects = append(objects, repository.PrivateObject{Key: item.Key, LastModified: item.LastModified})
	}
	return objects, nil
}
