package azurerepo

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AzureBlobStorageRepository struct {
	client               *azblob.Client
	sharedKeyCred        *azblob.SharedKeyCredential
	accountName          string
	containerName        string
	privateContainerName string
}

func New() (*AzureBlobStorageRepository, error) {
	accountName := os.Getenv("AZURE_STORAGE_ACCOUNT_NAME")
	accountKey := os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")
	containerName := os.Getenv("AZURE_STORAGE_CONTAINER_NAME")
	privateContainerName := os.Getenv("AZURE_STORAGE_PRIVATE_CONTAINER_NAME")
	if privateContainerName == "" {
		privateContainerName = containerName + "-private"
	}

	cred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create azure credential: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	client, err := azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create azure blob client: %w", err)
	}

	return &AzureBlobStorageRepository{
		client:               client,
		sharedKeyCred:        cred,
		accountName:          accountName,
		containerName:        containerName,
		privateContainerName: privateContainerName,
	}, nil
}

func (r *AzureBlobStorageRepository) PresignedPutURL(_ context.Context, objectKey string, _ string, expires time.Duration, _ int64) (string, error) {
	queryParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(expires),
		ContainerName: r.containerName,
		BlobName:      objectKey,
		Permissions:   (&sas.BlobPermissions{Write: true, Create: true}).String(),
	}.SignWithSharedKey(r.sharedKeyCred)
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}

	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s?%s",
		r.accountName, r.containerName, objectKey, queryParams.Encode()), nil
}

// StatObject は保存済み BLOB の実寸と Content-Type を返す。
// SAS ではサイズを縛れないので、受け入れ時にここで実物を測る
// （repository.StorageRepository のコメント参照）。
func (r *AzureBlobStorageRepository) StatObject(ctx context.Context, objectKey string) (repository.ObjectInfo, error) {
	blob := r.client.ServiceClient().NewContainerClient(r.containerName).NewBlobClient(objectKey)
	props, err := blob.GetProperties(ctx, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return repository.ObjectInfo{}, repository.ErrObjectNotFound
		}
		return repository.ObjectInfo{}, err
	}
	info := repository.ObjectInfo{}
	if props.ContentLength != nil {
		info.Size = *props.ContentLength
	}
	if props.ContentType != nil {
		info.ContentType = *props.ContentType
	}
	if props.ETag != nil {
		info.ETag = string(*props.ETag)
	}
	return info, nil
}

// azureCopyPollInterval / azureCopyPollTimeout は同一アカウント内コピーの完了待ち。
//
// 同一アカウント・同一コンテナのコピーは通常その場で終わり（応答の CopyStatus が
// success）、待ちには入らない。それでも Azure の契約上コピーは非同期なので、
// pending が返る可能性を捨てずに短く待つ。ここで待たずに成功として返すと、
// まだ中身の無いキーを DB に保存してしまう。
const (
	azureCopyPollInterval = 100 * time.Millisecond
	azureCopyPollTimeout  = 30 * time.Second
)

// azureCopySourceTTL はコピー元に付ける読み取りSASの寿命。
// サーバーからサーバーへの1回のコピーにしか使わないので短くてよい。
const azureCopySourceTTL = 5 * time.Minute

// CopyObject は同じコンテナ内で BLOB を写す。中身はこのプロセスを通らない。
//
// コピー元に読み取りSASを付けるのは、コンテナが公開かどうかに依らず動かすため。
// 公開コンテナ前提で素のURLを渡す実装にすると、非公開へ切り替えた瞬間に
// アップロードの受け入れだけが静かに壊れる。
//
// SourceIfMatch で「測ったときの中身と同じなら」という条件を付ける。一致しなければ
// Azure は SourceConditionNotMet を返し、何も書かない。
//
// 宛先側の条件は付けない。Azure 単体なら効かせられるが、もう一方の実装（MinIO）
// では効かないため、それを前提にすると保管先によって保証が変わる。宛先が一度きり
// しか書かれないことは呼び出し側で保証している（repository.StorageRepository 参照）。
func (r *AzureBlobStorageRepository) CopyObject(ctx context.Context, srcKey, dstKey, srcETag string) error {
	if srcETag == "" {
		// 条件を付けられないなら写さない（MinIO 実装と同じ理由）。
		return fmt.Errorf("refusing to copy %q without a source ETag", srcKey)
	}
	srcURL, err := r.readSASURL(srcKey)
	if err != nil {
		return err
	}

	etag := azcore.ETag(srcETag)
	dst := r.client.ServiceClient().NewContainerClient(r.containerName).NewBlobClient(dstKey)
	started, err := dst.StartCopyFromURL(ctx, srcURL, &blob.StartCopyFromURLOptions{
		SourceModifiedAccessConditions: &blob.SourceModifiedAccessConditions{SourceIfMatch: &etag},
	})
	if err != nil {
		if bloberror.HasCode(err, bloberror.SourceConditionNotMet, bloberror.ConditionNotMet) {
			return repository.ErrObjectChanged
		}
		if bloberror.HasCode(err, bloberror.BlobNotFound, bloberror.CannotVerifyCopySource) {
			return repository.ErrObjectNotFound
		}
		return err
	}
	if started.CopyStatus != nil && *started.CopyStatus == blob.CopyStatusTypeSuccess {
		return nil
	}

	deadline := time.Now().Add(azureCopyPollTimeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for the blob copy to finish")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(azureCopyPollInterval):
		}

		props, err := dst.GetProperties(ctx, nil)
		if err != nil {
			return err
		}
		if props.CopyStatus == nil {
			return nil
		}
		switch *props.CopyStatus {
		case blob.CopyStatusTypeSuccess:
			return nil
		case blob.CopyStatusTypePending:
			continue
		default:
			return fmt.Errorf("blob copy did not succeed: %s", *props.CopyStatus)
		}
	}
}

// readSASURL は1つの BLOB を読むためだけの短命な署名付きURLを作る。
func (r *AzureBlobStorageRepository) readSASURL(objectKey string) (string, error) {
	queryParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(azureCopySourceTTL),
		ContainerName: r.containerName,
		BlobName:      objectKey,
		Permissions:   (&sas.BlobPermissions{Read: true}).String(),
	}.SignWithSharedKey(r.sharedKeyCred)
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s?%s",
		r.accountName, r.containerName, objectKey, queryParams.Encode()), nil
}

func (r *AzureBlobStorageRepository) PublicURL(objectKey string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s",
		r.accountName, r.containerName, objectKey)
}

func (r *AzureBlobStorageRepository) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := r.client.DeleteBlob(ctx, r.containerName, objectKey, nil)
	return err
}

func (r *AzureBlobStorageRepository) PutPrivateObject(ctx context.Context, objectKey, _ string, body io.Reader, _ int64) error {
	_, err := r.client.UploadStream(ctx, r.privateContainerName, objectKey, body, nil)
	return err
}

func (r *AzureBlobStorageRepository) OpenPrivateObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	response, err := r.client.DownloadStream(ctx, r.privateContainerName, objectKey, nil)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (r *AzureBlobStorageRepository) DeletePrivateObject(ctx context.Context, objectKey string) error {
	_, err := r.client.DeleteBlob(ctx, r.privateContainerName, objectKey, nil)
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return nil
	}
	return err
}

func (r *AzureBlobStorageRepository) ListPrivateObjects(ctx context.Context, prefix string) ([]repository.PrivateObject, error) {
	pager := r.client.NewListBlobsFlatPager(r.privateContainerName, &azblob.ListBlobsFlatOptions{Prefix: &prefix})
	var objects []repository.PrivateObject
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name == nil || item.Properties == nil || item.Properties.LastModified == nil {
				continue
			}
			objects = append(objects, repository.PrivateObject{Key: *item.Name, LastModified: *item.Properties.LastModified})
		}
	}
	return objects, nil
}
