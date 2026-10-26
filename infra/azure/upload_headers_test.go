package azurerepo

import "testing"

// Azure Blob への PUT には x-ms-blob-type が必須。
// 欠けると 400 MissingRequiredHeader で、本文は一切保存されない（実機で確認）。
// ここを変えるとアップロードが全滅するので、値ごと固定しておく。
func TestUploadRequestHeadersDeclaresBlockBlob(t *testing.T) {
	got := (&AzureBlobStorageRepository{}).UploadRequestHeaders()
	if len(got) != 1 {
		t.Fatalf("ヘッダーが %d 個（1 個のはず）: %v", len(got), got)
	}
	if got["x-ms-blob-type"] != "BlockBlob" {
		t.Errorf("x-ms-blob-type が %q（\"BlockBlob\" のはず）", got["x-ms-blob-type"])
	}
}
