package graph

import (
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	"github.com/Cityboypenguin/SPACE-server/usecase/profile"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	GetUserByIDUseCase   user.GetUserByIDUseCase
	CreateUserUseCase    user.CreateUserUseCase
	ListUsersUseCase     user.ListUsersUseCase
	DeleteUserUseCase    user.DeleteUserUseCase
	UpdateUserUseCase    user.UpdateUserUseCase
	SearchUsersUseCase   user.SearchUsersUseCase
	LoginUserUseCase     user.LoginUserUseCase
	LogoutUserUseCase    user.LogoutUserUseCase
	UpdateProfileUseCase profile.UpdateProfileUseCase
	GetProfileUseCase    profile.GetProfileUseCase

	GetAdministratorByIDUseCase administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase  administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase   administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase  administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase  administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase   administrator.LoginAdministratorUseCase
	LogoutAdministratorUseCase  administrator.LogoutAdministratorUseCase
}
