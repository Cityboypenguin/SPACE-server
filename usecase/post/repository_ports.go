package post

import "github.com/Cityboypenguin/SPACE-server/repository"

// 投稿の書き込み系だけが、役割をまたいで広く触る。
//
// 1件引くだけのユースケースは repository.PostReader のように細い口を直接受け取る。
// ここに束ねた2つは、1つのトランザクションで本体・ハッシュタグ・メンションを
// まとめて書くので、どうしても複数の役割にまたがる。だからといって
// repository.PostRepository（全部入り）を受け取らせると、検索も一覧も触れる口を
// 渡すことになるので、必要な役割だけを束ねた名前を付けてある。

// postWriteRepository は投稿の作成・更新に要る口。
type postWriteRepository interface {
	repository.PostReader
	repository.PostWriter
	repository.HashtagRepository
	repository.PostMentionRepository
}

// postDeleteRepository は投稿の削除に要る口（消す前に持ち主を確かめるので読みも要る）。
type postDeleteRepository interface {
	repository.PostReader
	repository.PostWriter
}

// postMediaRepository は投稿の添付に要る口。作成では登録と紐づけ、更新では
// それに加えて既存の添付を外すので、3つの役割にまたがる。
type postMediaRepository interface {
	repository.MediaWriter
	repository.MediaAttachmentWriter
	repository.MediaDeleter
}
