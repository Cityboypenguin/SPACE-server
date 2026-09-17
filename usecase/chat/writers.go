package chat

import (
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/chat/internal/messagestore"
)

// MessageWriters は認可を前提とした保存処理（送信・編集・削除）の束。
//
// 中身のフィールドを非公開にしてあるのが肝。保存処理そのものは
// usecase/chat/internal/messagestore に居るので chat パッケージの外からは
// import できず、その唯一の受け渡し口であるこの型も Execute を外へ見せない。
// つまり配線（cmd/server/main.go）はこの束を組み立てて渡すことしかできず、
// 権限判定を飛ばして保存を直接叩く経路がコンパイル時に存在しなくなる。
//
// 以前は usecase/message の公開ユースケースを Deps に並べていたため、doc コメントで
// 「サービス経由で呼ぶこと」と書く以上の歯止めが無かった。
type MessageWriters struct {
	send   messagestore.SendMessageUseCase
	update messagestore.UpdateMessageUseCase
	delete messagestore.DeleteMessageUseCase
}

// NewMessageWriters は保存処理一式を組み立てる。
// messageRepository は MessageStore / MessageMentionStore の合成実装なので、
// 呼び出し側は必要な口だけを渡す（cmd/server/main.go 参照）。
func NewMessageWriters(
	store repository.MessageStore,
	mentionStore repository.MessageMentionStore,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) MessageWriters {
	return MessageWriters{
		send:   messagestore.NewSendMessageUseCase(store, mentionStore, mediaRepo, txManager),
		update: messagestore.NewUpdateMessageUseCase(store, mentionStore, txManager),
		delete: messagestore.NewDeleteMessageUseCase(store),
	}
}
