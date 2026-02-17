package graph

import "github.com/Cityboypenguin/SPACE-server/domain"

// 以前の var usersMemory, postsMemory は削除してください！

type Resolver struct {
	UserUsecase domain.UserUsecase
	PostUsecase domain.PostUsecase // 追加
}