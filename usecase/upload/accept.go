// Package upload は、アップロードされたオブジェクトを「受け入れてよいか」判定し、
// 保存してよいキーを返す口をユースケース層へ渡すためのもの。
//
// 判定の中身（上限・種別・所有者・署名付きURLの及ばないキーへの移送）は
// internal/upload にある。ここはそれをユースケースの依存として表すためだけの薄い層。
//
// なぜユースケース側に持たせるか。以前は受け入れがリゾルバの手順で、ユースケースの
// 契約には入っていなかった。つまり「オブジェクトキーを保存するユースケース」を
// 直接呼ぶ経路（別のAPI・バッチ・管理用コマンド）を足せば、検査を通さずに
// キーを保存できた。保存する側が受け入れを持てば、通さない道が無くなる。
package upload

import (
	"context"
	"errors"
	"time"
)

// ErrNoAcceptor は受け入れの実装が配線されていないことを指す。
//
// 実装が無いときに素通しにはしない。素通しは「検査も所有者確認も効いていない
// キーをそのまま保存する」ことで、受け入れをユースケースに持たせた目的そのものを
// 消す。配線を間違えたら、黙って通るのではなく必ず失敗する方がよい。
var ErrNoAcceptor = errors.New("upload acceptor is not configured")

// discardTimeout は後始末に与える時間。
//
// 後始末は呼び出し元の ctx が切れていても通したいが、期限まで外すと、
// 落ちかけのストレージ相手に要求の後片付けが居座り続ける。
const discardTimeout = 10 * time.Second

// Kind は「このユースケースが受け付けるアップロードの種別」。
//
// 値はオブジェクトキーの先頭セグメントで、internal/upload の表に対応する
// （対応が取れているかは internal/upload の kind_test.go が突き合わせている）。
// 種別を呼び出し側に言わせるのは、例えば自分のアバターのキーを投稿の添付として
// 申告する経路を塞ぐため。上限も用途も種別ごとに違う。
type Kind string

const (
	// Attachment は投稿・メッセージ・質問・回答の添付。
	Attachment    Kind = "media"
	Avatar        Kind = "avatars"
	CommunityIcon Kind = "community-icons"
	TermsDocument Kind = "terms"
)

// Acceptor は申告されたオブジェクトキーを受け入れ、保存してよいキーを返す。
//
// 返ってきたキーを保存すること。申告されたキーをそのまま保存すると、署名付きURLで
// 書き換えられる場所を指したままになる（internal/upload のパッケージコメント参照）。
//
// 空文字は「指定なし」としてそのまま返る。
//
// 誰のキーかは実装が ctx から決める。引数で受け取る形にすると、正しい値を渡す
// 責任が呼び出し側に残り、入口が増えたときにそこだけ取り違えられる。
type Acceptor interface {
	Accept(ctx context.Context, kind Kind, objectKey string) (string, error)
	// Discard は受け入れで公開したオブジェクトを取り消す。保存に失敗したときに
	// 呼ぶ（Session 参照）。消せなくても呼び出し元の失敗は変わらないので、
	// 結果は返さない。
	Discard(ctx context.Context, objectKey string)
}

// Session は1回の操作で受け入れたぶんを覚えておき、保存に失敗したときに
// 取り消せるようにする。
//
// 受け入れは1回ごとに新しいキーへ実体を写す。そのあとのDB書き込みが失敗すると、
// どこからも参照されない実体が残る。呼び出し側は次の2行を置くだけでよい。
//
//	uploads := uploadusecase.Begin(uc.uploads)
//	defer uploads.DiscardOnError(ctx, &err)
//
// 合図を「成功したら立てるフラグ」ではなく戻り値のエラーにしてあるのは、
// 立て忘れの結果が違いすぎるため。フラグ方式で立て忘れると、**成功したのに
// 添付を消す**。エラー方式で書き忘れても、残るのは参照されないオブジェクトだけ。
//
// 素通しだったキー（編集で送り返された、既に公開済みのもの）は覚えない。
// 見分けは「受け入れがキーを変えたかどうか」で付く。変わっていないキーは他の行が
// 参照している可能性があるので、消してはいけない。
type Session struct {
	a Acceptor
	// created はこの操作で新しく公開したキー。
	created []string
}

func Begin(a Acceptor) *Session { return &Session{a: a} }

var _ Acceptor = (*Session)(nil)

func (s *Session) Accept(ctx context.Context, kind Kind, objectKey string) (string, error) {
	if s == nil || s.a == nil {
		return "", ErrNoAcceptor
	}
	accepted, err := s.a.Accept(ctx, kind, objectKey)
	if err != nil {
		return "", err
	}
	if accepted != objectKey {
		s.created = append(s.created, accepted)
	}
	return accepted, nil
}

func (s *Session) Discard(ctx context.Context, objectKey string) {
	if s == nil || s.a == nil {
		return
	}
	s.a.Discard(ctx, objectKey)
}

// DiscardOnError は、操作がエラーで終わったときにこの操作で公開したものを
// 取り消す。名前付き戻り値の err を渡して defer で置く。
func (s *Session) DiscardOnError(ctx context.Context, err *error) {
	if s == nil || s.a == nil || err == nil || *err == nil {
		return
	}
	// 呼び出し元の ctx が既に切れていても片付けは通したい
	// （途中で諦めると、片付けが要る時ほど片付かない）。期限は付け直す。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), discardTimeout)
	defer cancel()
	for _, key := range s.created {
		s.a.Discard(ctx, key)
	}
	s.created = nil
}

// AcceptorFunc は関数をそのまま Acceptor にする。取り消しは何もしない。
type AcceptorFunc func(ctx context.Context, kind Kind, objectKey string) (string, error)

func (f AcceptorFunc) Accept(ctx context.Context, kind Kind, objectKey string) (string, error) {
	return f(ctx, kind, objectKey)
}

func (AcceptorFunc) Discard(context.Context, string) {}

// AcceptAll は添付のキーをまとめて受け入れ、保存してよいキーへ置き換えた写しを返す。
//
// 添付を受け取るユースケース（投稿・メッセージ・質問・回答）が同じことを
// 書かずに済むよう、ここに1つだけ置いてある。
//
// 同じキーが2回挙がっていたら、受け入れは1回だけにして同じ結果を使う。
// 受け入れは1回ごとに別の保存先を作る（internal/upload.Accept 参照）ので、
// 素直に2回呼ぶと同じ画像が2つ保存され、しかも1回目で元が消えるため
// 2回目は「置かれていない」で必ず失敗する。
func AcceptAll[T any](ctx context.Context, a Acceptor, kind Kind, items []T, key func(*T) *string) ([]T, error) {
	if a == nil {
		return nil, ErrNoAcceptor
	}
	if len(items) == 0 {
		return items, nil
	}
	accepted := make([]T, len(items))
	copy(accepted, items)
	seen := make(map[string]string, len(accepted))
	for i := range accepted {
		p := key(&accepted[i])
		if stored, ok := seen[*p]; ok {
			*p = stored
			continue
		}
		declared := *p
		stored, err := a.Accept(ctx, kind, declared)
		if err != nil {
			return nil, err
		}
		seen[declared] = stored
		*p = stored
	}
	return accepted, nil
}
