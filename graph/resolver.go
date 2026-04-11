package graph

import (
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	GetUserByIDUseCase         user.GetUserByIDUseCase
	CreateUserUseCase          user.CreateUserUseCase
	ListUsersUseCase           user.ListUsersUseCase
	DeleteUserUseCase          user.DeleteUserUseCase
	UpdateUserUseCase          user.UpdateUserUseCase
	SearchUsersUseCase         user.SearchUsersUseCase
	CreateAdministratorUseCase administrator.CreateAdministratorUseCase
}
