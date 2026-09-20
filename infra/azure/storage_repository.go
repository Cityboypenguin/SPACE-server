package azurerepo

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
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
	return info, nil
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
