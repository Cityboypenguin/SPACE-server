package azurerepo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// AzureBlobStorageRepository は署名する手段を2つのうちどちらか1つだけ持つ。
//
//   - sharedKeyCred: アカウントキー。そのストレージアカウントの中では全権で、
//     権限を絞れず、失効はキーのローテーションしか無く、操作ログに誰がやったかが
//     残らない。
//   - delegation: Entra ID の ID（アプリ登録やマネージド ID）から取る委任キー。
//     できることは割り当てたロールの範囲だけで、ロールを外せば止まり、操作ログに
//     その ID が残る。
//
// New がどちらを使うかを決める。署名の呼び出し側は sign を通すので、どちらでも
// 同じコードで動く。
type AzureBlobStorageRepository struct {
	client               *azblob.Client
	sharedKeyCred        *azblob.SharedKeyCredential
	delegation           *delegationKeyCache
	accountName          string
	containerName        string
	privateContainerName string
	// publicEndpoint は公開オブジェクトの URL を組むときの基点。CDN (Front Door) を
	// 前に置く場合にその配信ホストを入れる。空なら Blob の既定ホストを使う。
	// 末尾のスラッシュは New が落とす。
	publicEndpoint string
	// signPublicURLs は公開オブジェクトの URL に読み取りSASを付けるかどうか。
	//
	// コンテナーの匿名読み取りが閉じていると（ストレージアカウントの
	// allowBlobPublicAccess=false）、素の URL はブラウザから 403 になる。そこで
	// 読み取りだけを許す署名を付けて配る。
	//
	// ゼロ値が false なのは意図的で、この構造体を直に組む側は素の URL を見る。
	// New は既定で true を入れる。
	signPublicURLs bool
}

// azureDelegationKeyTTL は委任キーを要求する寿命（Azure の上限は7日）。
// azureDelegationKeyMargin は残りがこれを切ったら取り直す幅。
// azureDelegationKeySkew は開始時刻を過去へずらす幅。サーバー間の時計のずれで
// 「まだ有効になっていない鍵」を掴まないため。
//
// Margin は発行しうる SAS の最長寿命（azurePublicSASTTL + azurePublicSASWindow）より
// 必ず大きく取ること。小さいと SAS の失効が委任キーの失効を追い越し、署名は通るのに
// 使う時点で弾かれる URL を配ってしまう。sign が念のため上限を切り詰めるが、
// そこに頼ると URL が枠内で変わってキャッシュが効かなくなる。
const (
	azureDelegationKeyTTL    = 7 * 24 * time.Hour
	azureDelegationKeyMargin = 48 * time.Hour
	azureDelegationKeySkew   = 5 * time.Minute
)

// delegationKeyCache は Entra ID から取った委任キーを保持する。
//
// 委任キーの取得はネットワーク越しなので、SAS を1本作るたびに取りに行くわけには
// いかない（PublicURL は一覧の要素ごとに呼ばれる）。7日ぶんを1本取って使い回し、
// 残りが Margin を切ったときだけ取り直す。つまり実際の取得は数日に1回で、
// その1回を引いた呼び出しだけが待つ。
type delegationKeyCache struct {
	svc *service.Client

	mu     sync.Mutex
	udc    *service.UserDelegationCredential
	expiry time.Time
}

// get は有効な委任キーとその失効時刻を返す。必要なら取り直す。
func (c *delegationKeyCache) get(ctx context.Context) (*service.UserDelegationCredential, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.udc != nil && time.Until(c.expiry) > azureDelegationKeyMargin {
		return c.udc, c.expiry, nil
	}

	now := time.Now().UTC()
	expiry := now.Add(azureDelegationKeyTTL)
	udc, err := c.svc.GetUserDelegationCredential(ctx, service.KeyInfo{
		Start:  to.Ptr(now.Add(-azureDelegationKeySkew).Format(sas.TimeFormat)),
		Expiry: to.Ptr(expiry.Format(sas.TimeFormat)),
	}, nil)
	if err != nil {
		// 取り直しに失敗しても、手元の鍵がまだ生きているならそれで続ける。
		// Margin を 2 日取ってあるので、この猶予のあいだに復旧すれば表に影響は出ない。
		if c.udc != nil && now.Before(c.expiry) {
			return c.udc, c.expiry, nil
		}
		return nil, time.Time{}, fmt.Errorf("failed to get a user delegation key: %w", err)
	}

	c.udc, c.expiry = udc, expiry
	return c.udc, c.expiry, nil
}

// New はどちらの資格情報で署名するかを環境変数から決める。
//
// AZURE_STORAGE_ACCOUNT_KEY があればアカウントキー、無ければ Entra ID
// (DefaultAzureCredential)。キーの方を条件にしてあるのは、キーが設定から消えた
// ときに黙ってキー方式へ落ちないようにするため。
func New(ctx context.Context) (*AzureBlobStorageRepository, error) {
	accountName := os.Getenv("AZURE_STORAGE_ACCOUNT_NAME")
	accountKey := os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")
	containerName := os.Getenv("AZURE_STORAGE_CONTAINER_NAME")
	privateContainerName := os.Getenv("AZURE_STORAGE_PRIVATE_CONTAINER_NAME")
	if privateContainerName == "" {
		privateContainerName = containerName + "-private"
	}

	// CDN (Front Door) を前に置くと、ブラウザへ返す URL はその配信ホストを指す必要が
	// ある。Blob の既定ホストを返してしまうと CDN を通らず、置いた意味が無くなる。
	//
	// 未設定なら Blob の既定ホストへ素で当てる。つまり CDN 無しの構成もそのまま動く。
	// アップロードや SAS の発行は常に Blob の本来のホスト(serviceURL)を使うので、
	// ここを変えても影響しない。署名は配信ホストでは検証できないため。
	//
	// S3 側の MINIO_PUBLIC_ENDPOINT と同じ役割で、パスの形も揃えてある
	// (<endpoint>/<container>/<key>)。CDN 側は受けたパスをそのまま Blob へ
	// 転送すればよい。
	publicEndpoint := strings.TrimRight(os.Getenv("AZURE_BLOB_PUBLIC_ENDPOINT"), "/")

	// 既定で署名を付ける。匿名読み取りを開けるのはコントロールプレーンの設定
	// (allowBlobPublicAccess) で、アカウントキーでは開けられない。つまり「鍵は
	// 持っているのに画像だけ表示されない」が既定の状態なので、素の URL を配る方を
	// 特別扱いにする。
	//
	// コンテナーを本当に公開にした場合、またはクエリ文字列を転送しない CDN を前に
	// 置いた場合だけ AZURE_BLOB_PUBLIC_SIGN=false で素の URL に戻す。
	signPublicURLs := !strings.EqualFold(strings.TrimSpace(os.Getenv("AZURE_BLOB_PUBLIC_SIGN")), "false")

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)

	var (
		client        *azblob.Client
		sharedKeyCred *azblob.SharedKeyCredential
		delegation    *delegationKeyCache
	)
	if accountKey != "" {
		cred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure credential: %w", err)
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure blob client: %w", err)
		}
		sharedKeyCred = cred
	} else {
		// DefaultAzureCredential は AZURE_TENANT_ID / AZURE_CLIENT_ID /
		// AZURE_CLIENT_SECRET → マネージド ID → 手元の az login の順に探す。
		// だから本番（アプリ登録）、Azure 上の VM（マネージド ID、シークレット不要）、
		// 開発者の手元が、同じコードと同じ設定で動く。
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure identity credential: %w", err)
		}
		client, err = azblob.NewClient(serviceURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create azure blob client: %w", err)
		}
		delegation = &delegationKeyCache{svc: client.ServiceClient()}

		// 起動時に1本引いておく。ここで確かめないと、ロールの割り当て漏れや
		// シークレットの誤りが「最初に画像を上げた人が失敗する」形で出る。
		if _, _, err := delegation.get(ctx); err != nil {
			return nil, err
		}
	}

	return &AzureBlobStorageRepository{
		client:               client,
		sharedKeyCred:        sharedKeyCred,
		delegation:           delegation,
		accountName:          accountName,
		containerName:        containerName,
		privateContainerName: privateContainerName,
		publicEndpoint:       publicEndpoint,
		signPublicURLs:       signPublicURLs,
	}, nil
}

// errNoSigningCredential は署名する手段がどちらも無いときに返る。
var errNoSigningCredential = errors.New("no credential is available to sign a SAS")

// canSign は署名できるかを、ネットワークにも context にも触らずに判定する。
func (r *AzureBlobStorageRepository) canSign() bool {
	return r.sharedKeyCred != nil || r.delegation != nil
}

// sign は SAS に署名する。アカウントキーと委任キーの違いをここだけに閉じ込める。
//
// アカウントキー方式では純粋なローカル計算で、ctx は使わない。委任キー方式では
// 鍵の取り直しが要るときだけネットワークへ出る（delegationKeyCache 参照）。
func (r *AzureBlobStorageRepository) sign(ctx context.Context, values sas.BlobSignatureValues) (sas.QueryParameters, error) {
	if r.sharedKeyCred != nil {
		return values.SignWithSharedKey(r.sharedKeyCred)
	}
	if r.delegation == nil {
		return sas.QueryParameters{}, errNoSigningCredential
	}

	udc, keyExpiry, err := r.delegation.get(ctx)
	if err != nil {
		return sas.QueryParameters{}, err
	}
	// SAS は委任キーより長生きできない。追い越すと、署名は通るのに使う時点で
	// 弾かれる URL ができる。Margin の取り方が正しければここは効かない（保険）。
	if values.ExpiryTime.After(keyExpiry) {
		values.ExpiryTime = keyExpiry
	}
	return values.SignWithUserDelegation(udc)
}

func (r *AzureBlobStorageRepository) PresignedPutURL(ctx context.Context, objectKey string, _ string, expires time.Duration, _ int64) (string, error) {
	queryParams, err := r.sign(ctx, sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(expires),
		ContainerName: r.containerName,
		BlobName:      objectKey,
		Permissions:   (&sas.BlobPermissions{Write: true, Create: true}).String(),
	})
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
	srcURL, err := r.readSASURL(ctx, srcKey)
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
func (r *AzureBlobStorageRepository) readSASURL(ctx context.Context, objectKey string) (string, error) {
	queryParams, err := r.sign(ctx, sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(azureCopySourceTTL),
		ContainerName: r.containerName,
		BlobName:      objectKey,
		Permissions:   (&sas.BlobPermissions{Read: true}).String(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s?%s",
		r.accountName, r.containerName, objectKey, queryParams.Encode()), nil
}

// azurePublicSASTTL は公開オブジェクトに付ける読み取りSASの最短の寿命。
// azurePublicSASWindow は失効時刻を丸める幅。
//
// 失効時刻を丸めるのは、同じオブジェクトに対して同じ URL を返し続けるため。
// 毎回 now+TTL で署名すると URL が呼び出しごとに変わり、ブラウザも CDN も
// 別物として扱うのでキャッシュが一切効かなくなる（同じ画像を何度も取り直す）。
//
// 丸めた結果、実際の寿命は TTL から TTL+Window の幅を取る。失効の直前に掴んだ
// URL でも最低 TTL は使えるので、開いたままのページで画像が切れることはない。
// azurePublicSignTimeout は PublicURL が委任キーを取り直すときの待ちの上限。
const (
	azurePublicSASTTL      = 24 * time.Hour
	azurePublicSASWindow   = time.Hour
	azurePublicSignTimeout = 5 * time.Second
)

// PublicURL は公開オブジェクトをブラウザへ渡すための URL を組む。
//
// 戻り値は DB に保存していない（DB が持つのは storage_key だけ）。読み出しのたびに
// ここで組み立てるので、CDN の追加や撤去、ストレージアカウントの移行、署名の
// 有無の切り替えをしても既存データの書き換えは要らない。
//
// signPublicURLs が立っていれば読み取りSASを付ける。コンテナーの匿名読み取りが
// 閉じている構成では、これが無いとブラウザからの取得が 403 になる。
func (r *AzureBlobStorageRepository) PublicURL(objectKey string) string {
	base := r.publicBaseURL(objectKey)
	if !r.signPublicURLs || !r.canSign() {
		return base
	}

	// PublicURL は repository.StorageRepository の契約上 error を返さないので、
	// 委任キーの取り直しが要る回だけここで待つ。数日に1回しか起きないが、
	// 読み出しの経路で長く止まるのは避けたいので短く切る。
	ctx, cancel := context.WithTimeout(context.Background(), azurePublicSignTimeout)
	defer cancel()

	queryParams, err := r.sign(ctx, sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    publicSASExpiry(time.Now()),
		ContainerName: r.containerName,
		BlobName:      objectKey,
		Permissions:   (&sas.BlobPermissions{Read: true}).String(),
	})
	if err != nil {
		// 読み出し全体を落とすより、素の URL を返して「画像が出ない」に留める
		// （コンテナーが公開ならそれで足りる）。
		return base
	}
	return base + "?" + queryParams.Encode()
}

// publicBaseURL は署名を除いた公開 URL。CDN を前に置いたときだけ配信ホストを向く。
func (r *AzureBlobStorageRepository) publicBaseURL(objectKey string) string {
	if r.publicEndpoint != "" {
		return fmt.Sprintf("%s/%s/%s", r.publicEndpoint, r.containerName, objectKey)
	}
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s",
		r.accountName, r.containerName, objectKey)
}

// publicSASExpiry は now を含む時間枠の全員に共通の失効時刻を返す。
// 同じ枠のあいだ署名が変わらないので、URL がキャッシュ可能になる。
func publicSASExpiry(now time.Time) time.Time {
	return now.UTC().Truncate(azurePublicSASWindow).Add(azurePublicSASWindow + azurePublicSASTTL)
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
