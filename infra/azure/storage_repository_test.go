package azurerepo

import "testing"

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
