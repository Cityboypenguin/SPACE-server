package question

import "github.com/Cityboypenguin/SPACE-server/repository"

// mediaAttachRepository は添付を保存する経路に要る口
// （メディア本体を登録し、それを question へ紐づける）。
type mediaAttachRepository interface {
	repository.MediaWriter
	repository.MediaAttachmentWriter
}

// mediaReplaceRepository は添付を差し替える経路に要る口
// （いま付いているものを引き、外す）。
type mediaReplaceRepository interface {
	repository.MediaReader
	repository.MediaDeleter
}
