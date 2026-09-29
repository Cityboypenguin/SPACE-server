package azurerepo

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// PublicURL は CDN を前に置いたときだけ配信ホストを向く。未設定なら Blob の
// 既定ホストへ素で当てる。CDN の有無で URL の「パスの形」は変わらないこと
// （CDN 側は受けたパスをそのまま Blob へ転送するだけでよい）。
func TestPublicURL(t *testing.T) {
	tests := []struct {
		name           string
		publicEndpoint string
		want           string
	}{
		{
			name:           "CDN 無し: Blob の既定ホスト",
			publicEndpoint: "",
			want:           "https://acct.blob.core.windows.net/space-avatars/avatars/1/a.webp",
		},
		{
			name:           "CDN 有り: 配信ホストを向く",
			publicEndpoint: "https://img.senshu-universe.com",
			want:           "https://img.senshu-universe.com/space-avatars/avatars/1/a.webp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AzureBlobStorageRepository{
				accountName:    "acct",
				containerName:  "space-avatars",
				publicEndpoint: tt.publicEndpoint,
			}
			if got := r.PublicURL("avatars/1/a.webp"); got != tt.want {
				t.Errorf("PublicURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// 匿名読み取りが閉じているコンテナーでは、PublicURL が読み取りSASを付けないと
// ブラウザからの取得が 403 になる。署名の有無は signPublicURLs だけで決まり、
// パスの形（CDN 側がそのまま Blob へ転送できること）は変わらないこと。
func TestPublicURLSigned(t *testing.T) {
	cred, err := azblob.NewSharedKeyCredential("acct", base64.StdEncoding.EncodeToString([]byte("test-key")))
	if err != nil {
		t.Fatalf("NewSharedKeyCredential() error = %v", err)
	}
	r := &AzureBlobStorageRepository{
		accountName:    "acct",
		containerName:  "space-avatars",
		sharedKeyCred:  cred,
		signPublicURLs: true,
	}

	got := r.PublicURL("avatars/1/a.webp")
	base := "https://acct.blob.core.windows.net/space-avatars/avatars/1/a.webp"
	if !strings.HasPrefix(got, base+"?") {
		t.Fatalf("PublicURL() = %q, want it to start with %q", got, base+"?")
	}

	query, err := url.ParseQuery(strings.TrimPrefix(got, base+"?"))
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	// 読み取りだけを許すこと。書き込みが混ざると、URL を渡した相手に上書きを許す。
	if perm := query.Get("sp"); perm != "r" {
		t.Errorf("sp = %q, want %q", perm, "r")
	}
	if query.Get("sig") == "" {
		t.Error("sig is missing: the URL is not signed")
	}
	if query.Get("se") == "" {
		t.Error("se is missing: the URL never expires")
	}
}

// 署名は資格情報が無ければ付けられない。構造体を直に組む側（テスト）が
// panic せず素の URL を見ること。
func TestPublicURLSignedWithoutCredential(t *testing.T) {
	r := &AzureBlobStorageRepository{
		accountName:    "acct",
		containerName:  "space-avatars",
		signPublicURLs: true,
	}
	want := "https://acct.blob.core.windows.net/space-avatars/avatars/1/a.webp"
	if got := r.PublicURL("avatars/1/a.webp"); got != want {
		t.Errorf("PublicURL() = %q, want %q", got, want)
	}
}

// 同じ時間枠のあいだ失効時刻が動かないこと。ここが呼び出しごとに変わると URL も
// 変わり、ブラウザと CDN のキャッシュが効かなくなる（同じ画像を毎回取り直す）。
// 枠をまたげば必ず先へ進み、どの時点で掴んだ URL も最低 TTL は使えること。
func TestPublicSASExpiry(t *testing.T) {
	base := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)

	head := publicSASExpiry(base)
	tail := publicSASExpiry(base.Add(azurePublicSASWindow - time.Nanosecond))
	if !head.Equal(tail) {
		t.Errorf("同じ枠で失効時刻が動いた: %v != %v", head, tail)
	}

	next := publicSASExpiry(base.Add(azurePublicSASWindow))
	if !next.After(head) {
		t.Errorf("枠をまたいだのに失効時刻が進んでいない: %v <= %v", next, head)
	}

	// 枠の終わり際に掴んだ URL でも TTL は残っていること。
	last := base.Add(azurePublicSASWindow - time.Nanosecond)
	if remaining := publicSASExpiry(last).Sub(last); remaining < azurePublicSASTTL {
		t.Errorf("寿命が TTL を下回った: %v < %v", remaining, azurePublicSASTTL)
	}
}
