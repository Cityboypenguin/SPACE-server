package profile

import "github.com/Cityboypenguin/SPACE-server/repository"

// profileUserRepository はプロフィール更新に要る口。
// 公開列だけを読み書きする（連絡先にも認証情報にも触らない）。
type profileUserRepository interface {
	repository.UserReader
	repository.UserWriter
}
