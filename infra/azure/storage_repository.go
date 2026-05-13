package azurerepo

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
)

type AzureBlobStorageRepository struct {
	client        *azblob.Client
	sharedKeyCred *azblob.SharedKeyCredential
	accountName   string
	containerName string
}

func New() (*AzureBlobStorageRepository, error) {
	accountName   := os.Getenv("AZURE_STORAGE_ACCOUNT_NAME")
	accountKey    := os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")
	containerName := os.Getenv("AZURE_STORAGE_CONTAINER_NAME")

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
		client:        client,
		sharedKeyCred: cred,
		accountName:   accountName,
		containerName: containerName,
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

func (r *AzureBlobStorageRepository) PublicURL(objectKey string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s",
		r.accountName, r.containerName, objectKey)
}

func (r *AzureBlobStorageRepository) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := r.client.DeleteBlob(ctx, r.containerName, objectKey, nil)
	return err
}
