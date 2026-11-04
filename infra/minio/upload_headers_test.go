package miniorepo

import "testing"

// S3 互換の署名付き PUT に追加ヘッダーは要らない。
// 署名に含まれないヘッダーを足すと署名が合わなくなる場合があるので、
// 「何も足さない」ことを固定しておく。
func TestUploadRequestHeadersDeclaresNothing(t *testing.T) {
	if got := (&MinIOStorageRepository{}).UploadRequestHeaders(); len(got) != 0 {
		t.Errorf("ヘッダーが %v（空のはず）", got)
	}
}
