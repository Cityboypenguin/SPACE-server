package graph

import (
	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
)

// NewPubSubCodec は subscription に流れる型を登録した Codec を返す。
//
// 台をまたぐ配信では、届くのがバイト列だけになるので「これは何の型か」を
// 送る側が書いておかなければならない（理由は pubsub.Codec のコメント）。
//
// 名前は Go の型名と切り離してある。型を別パッケージへ移したりリネームしたときに、
// 配信だけが静かに壊れる（そのトピックに何も届かなくなる）のを避けるため。
// 名前を変えてよいのは、全台を同時に入れ替えられるときだけ。
//
// ここに足し忘れると Publish がエラーを残して配信しない。黙って落ちるよりは
// ログに出る方がよいが、subscription を1つ足すたびにここも見ること。
func NewPubSubCodec() *pubsub.Codec {
	c := pubsub.NewCodec()
	c.Register("message", (*gqlmodel.Message)(nil))
	c.Register("question", (*gqlmodel.Question)(nil))
	c.Register("answer", (*gqlmodel.Answer)(nil))
	c.Register("poll", (*gqlmodel.Poll)(nil))
	c.Register("room_read_status", (*gqlmodel.RoomReadStatusUpdate)(nil))
	// 取り込み状況だけは値で流している（Publish 側が courseimport.Status を
	// そのまま渡す）ので、値の型で登録する。ここをポインタにすると、受け手の
	// 型アサーションが通らずこのトピックだけ届かなくなる。
	c.Register("course_import_status", courseimport.Status{})
	return c
}
