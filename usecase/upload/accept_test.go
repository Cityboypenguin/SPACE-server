package upload

import (
	"context"
	"errors"
	"testing"
)

type item struct{ key string }

// countingAcceptor は「受け入れは1回ごとに別の保存先を作る」本物の性質を真似る。
// 呼ばれるたびに違うキーを返し、2回目以降は元がもう無い（＝失敗する）。
type countingAcceptor struct {
	calls     map[string]int
	n         int
	discarded []string
}

func (a *countingAcceptor) Accept(_ context.Context, _ Kind, objectKey string) (string, error) {
	if a.calls == nil {
		a.calls = map[string]int{}
	}
	a.calls[objectKey]++
	if a.calls[objectKey] > 1 {
		return "", errors.New("アップロードされていないファイルが指定されました")
	}
	a.n++
	return "accepted/" + objectKey + "#" + string(rune('a'+a.n-1)), nil
}

func (a *countingAcceptor) Discard(_ context.Context, objectKey string) {
	a.discarded = append(a.discarded, objectKey)
}

// TestAcceptAll_AcceptsEachDeclaredKeyOnce は、同じキーが2回挙がっても
// 受け入れが1回で済むことを確かめる。
//
// 受け入れは1回ごとに別の保存先を作るので、素直に2回呼ぶと同じ画像が2つ保存され、
// しかも元は1回目で消えているので2回目は必ず失敗する。
func TestAcceptAll_AcceptsEachDeclaredKeyOnce(t *testing.T) {
	a := &countingAcceptor{}
	items := []item{{key: "staging/media/42/x.png"}, {key: "staging/media/42/x.png"}, {key: "staging/media/42/y.png"}}

	got, err := AcceptAll(context.Background(), a, Attachment, items, func(i *item) *string { return &i.key })
	if err != nil {
		t.Fatalf("AcceptAll: %v", err)
	}
	if a.calls["staging/media/42/x.png"] != 1 {
		t.Fatalf("同じキーを %d 回受け入れた", a.calls["staging/media/42/x.png"])
	}
	if got[0].key != got[1].key {
		t.Fatalf("同じキーが別々の保存先になった: %q / %q", got[0].key, got[1].key)
	}
	if got[2].key == got[0].key {
		t.Fatalf("別のキーが同じ保存先になった: %q", got[2].key)
	}
	// 元の並びは書き換えないこと（呼び出し側が渡した値をそのまま使う経路がある）。
	if items[0].key != "staging/media/42/x.png" {
		t.Fatalf("入力が書き換わった: %q", items[0].key)
	}
}

// TestSession_DiscardsWhatItPublishedWhenTheSaveFails は今回の本体。
//
// 受け入れは1回ごとに新しいキーへ実体を写す。そのあとの保存が失敗したとき、
// 取り消さないと、どこからも参照されない実体がそのキーに残り続ける。
// 以前は最終キーが staging の名前から導けたので、やり直した要求が同じキーを
// 拾い直していた。キーが毎回変わるぶん、取り消しは明示的にやる必要がある。
func TestSession_DiscardsWhatItPublishedWhenTheSaveFails(t *testing.T) {
	a := &countingAcceptor{}
	s := Begin(a)

	accepted, err := s.Accept(context.Background(), Attachment, "staging/media/42/x.png")
	if err != nil {
		t.Fatal(err)
	}

	// 保存が失敗したまま抜ける。
	failure := errors.New("db is down")
	s.DiscardOnError(context.Background(), &failure)

	if len(a.discarded) != 1 || a.discarded[0] != accepted {
		t.Fatalf("discarded = %v, want [%q]", a.discarded, accepted)
	}
}

// 保存が成立したら消さないこと。
func TestSession_KeepsWhatItPublishedWhenTheSaveSucceeds(t *testing.T) {
	a := &countingAcceptor{}
	s := Begin(a)

	if _, err := s.Accept(context.Background(), Attachment, "staging/media/42/x.png"); err != nil {
		t.Fatal(err)
	}

	var success error
	s.DiscardOnError(context.Background(), &success)

	if len(a.discarded) != 0 {
		t.Fatalf("discarded = %v, want 保存できたものは消さない", a.discarded)
	}
}

// 素通しだったキー（編集で送り返された、既に公開済みのもの）は消さないこと。
// 他の行が参照しているので、消すと既存の投稿から画像が消える。
func TestSession_NeverDiscardsKeysItDidNotPublish(t *testing.T) {
	// 受け入れがキーを変えない＝素通し。
	passthrough := AcceptorFunc(func(_ context.Context, _ Kind, objectKey string) (string, error) {
		return objectKey, nil
	})
	a := &recordingDiscards{Acceptor: passthrough}
	s := Begin(a)

	if _, err := s.Accept(context.Background(), Attachment, "media/42/already-published.png"); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("db is down")
	s.DiscardOnError(context.Background(), &failure)

	if len(a.discarded) != 0 {
		t.Fatalf("discarded = %v, want 素通しのキーは消さない", a.discarded)
	}
}

type recordingDiscards struct {
	Acceptor
	discarded []string
}

func (r *recordingDiscards) Discard(_ context.Context, objectKey string) {
	r.discarded = append(r.discarded, objectKey)
}

// 呼び出し元の ctx が既に切れていても片付けは通すこと。
// 途中で諦めると、片付けが要るときほど片付かない。
func TestSession_CleansUpEvenWhenTheRequestWasCancelled(t *testing.T) {
	a := &countingAcceptor{}
	s := Begin(a)

	ctx, cancel := context.WithCancel(context.Background())
	if _, err := s.Accept(ctx, Attachment, "staging/media/42/x.png"); err != nil {
		t.Fatal(err)
	}
	cancel()

	failure := errors.New("db is down")
	s.DiscardOnError(ctx, &failure)

	if len(a.discarded) != 1 {
		t.Fatalf("discarded = %v, want 1 件", a.discarded)
	}
}

// TestSession_FailsClosedWithoutAnAcceptor は、受け入れの実装が配線されていない
// ときに黙って素通ししないことを確かめる。
//
// 素通しすると、検査も所有者確認も効いていないキーがそのまま保存される。
// 「ユースケースを通せば必ず検査される」という保証が、配線の間違いひとつで
// エラーも出さずに消えるのは避けたい。
func TestSession_FailsClosedWithoutAnAcceptor(t *testing.T) {
	s := Begin(nil)

	got, err := s.Accept(context.Background(), Attachment, "staging/media/42/x.png")
	if !errors.Is(err, ErrNoAcceptor) {
		t.Fatalf("error = %v, want %v", err, ErrNoAcceptor)
	}
	if got != "" {
		t.Fatalf("key = %q, want 空（保存させないこと）", got)
	}
}

func TestAcceptAll_FailsClosedWithoutAnAcceptor(t *testing.T) {
	items := []item{{key: "staging/media/42/x.png"}}

	got, err := AcceptAll(context.Background(), nil, Attachment, items, func(i *item) *string { return &i.key })
	if !errors.Is(err, ErrNoAcceptor) {
		t.Fatalf("error = %v, want %v", err, ErrNoAcceptor)
	}
	if got != nil {
		t.Fatalf("items = %v, want nil（保存させないこと）", got)
	}
}

// 後始末には期限が付いていること。
// 期限まで外すと、落ちかけのストレージ相手に後片付けが居座り続ける。
func TestSession_DiscardHasADeadline(t *testing.T) {
	var deadlineSet bool
	a := &deadlineProbe{onDiscard: func(ctx context.Context) {
		_, deadlineSet = ctx.Deadline()
	}}
	s := Begin(a)
	if _, err := s.Accept(context.Background(), Attachment, "staging/media/42/x.png"); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("db is down")
	s.DiscardOnError(context.Background(), &failure)

	if !deadlineSet {
		t.Fatal("後始末の context に期限が付いていない")
	}
}

type deadlineProbe struct{ onDiscard func(context.Context) }

func (deadlineProbe) Accept(_ context.Context, _ Kind, objectKey string) (string, error) {
	return "accepted/" + objectKey, nil
}

func (p *deadlineProbe) Discard(ctx context.Context, _ string) { p.onDiscard(ctx) }
