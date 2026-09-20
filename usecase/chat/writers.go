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
// つまり配線（cmd/server/main.go）はこの束を組み立てて渡すことしかできない。
//
// 以前は usecase/message の公開ユースケースを Deps に並べていたため、doc コメントで
// 「サービス経由で呼ぶこと」と書く以上の歯止めが無かった。
//
// # 何が塞がっていて、何が塞がっていないか
//
// 塞げているのは「保存ユースケースへの到達」だけで、書き込みそのものではない。
// 正直に書くと:
//
//   - 塞がっている: chat パッケージ外のコードが messagestore の Send/Update/Delete
//     ユースケースを組み立てる・呼ぶこと（internal パッケージ規則 + 非公開フィールド）。
//     権限判定を飛ばして「メッセージ送信の一式」を再現する近道は作れない。
//   - 塞がっていない(1): リポジトリの書き込み口。repository.MessageWriter
//     （SaveMessage / UpdateMessage / SoftDeleteMessage）は公開インターフェースで、
//     import しようと思えばどこからでもできる。これを防いでいるのは型ではなく
//     配線の規律で、composition root (cmd/server/main.go) がこの口を渡す先を
//     NewMessageWriters だけに絞っている。他のユースケースには
//     repository.MessageReader しか渡さないので、書き込みを「依存として受け取る」
//     ことはできない。破るには自分で実装を掴みに行く必要があり、その差分は
//     composition root に現れるのでレビューで見える。
//   - 塞がっていない(2): usecase/chat パッケージ自身。同じパッケージなら非公開
//     フィールドにも触れるし、messagestore も import できる。ここは「認可判定を
//     持っている側」なので当然だが、chat パッケージに新しいコードを足すときは
//     AccessPolicy を通す責任が人間側に残る。
//   - 塞がっていない(3): DB への直接アクセス（infra/mysql や生 SQL）。
//
// つまりこれは「うっかり別の層から保存を呼んでしまう」事故を型で潰す仕組みで、
// 悪意ある回避を防ぐ壁ではない。
type MessageWriters struct {
	send   messagestore.SendMessageUseCase
	update messagestore.UpdateMessageUseCase
	delete messagestore.DeleteMessageUseCase
}

// NewMessageWriters は保存処理一式を組み立てる。
//
// writer（repository.MessageWriter）を渡してよいのはこの関数だけ、というのが
// 配線側の規律（上の「塞がっていない(1)」参照）。reader は返信先の確認と編集前の
// 取得に使うだけなので、書き込みと分けて受け取る。
func NewMessageWriters(
	reader repository.MessageReader,
	writer repository.MessageWriter,
	mentionStore repository.MessageMentionStore,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) MessageWriters {
	return MessageWriters{
		send:   messagestore.NewSendMessageUseCase(reader, writer, mentionStore, mediaRepo, txManager),
		update: messagestore.NewUpdateMessageUseCase(reader, writer, mentionStore, txManager),
		delete: messagestore.NewDeleteMessageUseCase(writer),
	}
}
