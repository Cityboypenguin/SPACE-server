// Package messagestore はチャットメッセージの「保存処理」（送信・編集・削除）。
//
// なぜ usecase/chat/internal/ に置いているか:
//
// これらは認可を一切持たない。授業ルームの学期・履修、room_users の membership、
// DM のブロック判定、所有権、匿名IDの採番は全て usecase/chat 側の判定で、ここを
// 直接呼べばそれらが丸ごとバイパスされる。以前は usecase/message にあり
// 「サービス経由で呼ぶこと」という doc コメントだけが歯止めだったので、別の
// リゾルバやバッチが素直に呼んでしまえば認可なしで書き込めてしまった。
//
// Go の internal パッケージ規則により、ここを import できるのは
// usecase/chat/... 配下だけになる。つまり「保存ユースケースへ到達するには
// usecase/chat を通る」という形が、コメントではなくコンパイラで決まる。
//
// ただしこれで書き込みそのものが塞がるわけではない: repository.MessageWriter は
// 公開インターフェースなので、リポジトリの書き込み口はここを通さずとも呼べる。
// そちらは composition root がこの口を NewMessageWriters にしか渡さないという
// 配線の規律で守っている。何が塞げて何が塞げないかは usecase/chat/writers.go の
// コメントに全部書いてある。
//
// 読み取り系（GetMessageByID・一覧取得・メンション取得など）は認可を前提としない
// ／リゾルバや DataLoader から直接使うため、usecase/message に公開のまま残してある。
//
// 保存処理を新しく足すときもここへ置くこと。外から呼びたくなったら、それは
// usecase/chat に認可付きの入口を足すべきサイン。
package messagestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// SendMessageUseCase はメッセージ1件の「保存処理」。
//
// 権限判定はここには無い（授業ルームの学期・履修、room_users の membership、
// DM のブロック判定、匿名IDの採番は usecase/chat の送信サービスが持つ）。
// このパッケージが internal に居るおかげで、認可を通さずこのユースケースへ
// 辿り着く経路はコンパイル時に塞がれている（リポジトリの書き込み口まで塞がる
// わけではない。パッケージのコメント参照）。
type SendMessageUseCase interface {
	// replyToID を渡すと引用返信になる。返信先は同じルームの未削除メッセージである
	// 必要があり、そうでなければエラーになる。
	// mentions は ResolveMentionsUseCase で検証済みのメンション（コミュニティのみ）。
	Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error)
}

var _ SendMessageUseCase = &SendMessageInteractor{}

// メッセージ本体とメンション行は関心が違うので、それぞれの狭い口を受け取る。
// 本体側は読み (返信先の確認) と書き (保存) を分けて受け取る: 書き込みの口
// (repository.MessageWriter) を配線で渡す先をここだけに絞るための分割で、
// 意図は repository.MessageWriter と usecase/chat/writers.go のコメント参照。
type SendMessageInteractor struct {
	reader       repository.MessageReader
	writer       repository.MessageWriter
	mentionStore repository.MessageMentionStore
	mediaRepo    repository.MediaRepository
	txManager    repository.TxManager
}

func NewSendMessageUseCase(
	reader repository.MessageReader,
	writer repository.MessageWriter,
	mentionStore repository.MessageMentionStore,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) SendMessageUseCase {
	return &SendMessageInteractor{
		reader:       reader,
		writer:       writer,
		mentionStore: mentionStore,
		mediaRepo:    mediaRepo,
		txManager:    txManager,
	}
}

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error) {
	content = strings.TrimSpace(content)
	if err := validateContent(content); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("media/%d/", userID)
	mediaKeys := make([]string, 0, len(mediaInputs))
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
		mediaKeys = append(mediaKeys, input.StorageKey)
	}

	// 返信先は同じルームの、まだ削除されていないメッセージだけ許す。
	// （他ルームのメッセージIDを指定して内容を覗き見られるのを防ぐ）
	if replyToID != nil {
		parent, err := uc.reader.GetMessageByID(ctx, *replyToID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
		if !parent.IsInRoom(roomID) {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
	}

	now := time.Now()
	// 「本文も添付も空」の検証は model.NewMessage が持つ（生成経路が増えても漏れないように）。
	m, err := model.NewMessage(model.CreateMessageParam{
		RoomID:    roomID,
		UserID:    userID,
		ReplyToID: replyToID,
		Content:   content,
		MediaKeys: mediaKeys,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	m.Mentions = mentions

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.writer.SaveMessage(ctx, m); err != nil {
			return err
		}
		if err := uc.mentionStore.CreateMessageMentions(ctx, m.ID, mentions); err != nil {
			return err
		}
		// 添付はまとめて保存する（media 行を1本、紐付けを1本）。以前は入力1件ごとに
		// 2往復していたので、4枚付けると8往復していた。
		if medias := model.NewMediaBatch(userID, mediaInputs, now); len(medias) > 0 {
			if err := uc.mediaRepo.CreateMediaBatch(ctx, medias); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreateMessageMediaBatch(ctx, m.ID, model.MediaIDs(medias), 0); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return m, nil
}
