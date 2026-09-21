package messagestore

import "github.com/Cityboypenguin/SPACE-server/repository"

// mediaAttachRepository は送信時に添付を保存する口
// （メディア本体を登録し、それをメッセージへ紐づける）。
type mediaAttachRepository interface {
	repository.MediaWriter
	repository.MediaAttachmentWriter
}
