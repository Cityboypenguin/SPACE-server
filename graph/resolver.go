package graph

import (
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	GetUserByIDUseCase user.GetUserByIDUseCase
	CreateUserUseCase  user.CreateUserUseCase
	ListUsersUseCase   user.ListUsersUseCase
	DeleteUserUseCase  user.DeleteUserUseCase
	UpdateUserUseCase  user.UpdateUserUseCase
	SearchUsersUseCase user.SearchUsersUseCase
	LoginUserUseCase   user.LoginUserUseCase
	LogoutUserUseCase  user.LogoutUserUseCase

	GetAdministratorByIDUseCase administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase  administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase   administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase  administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase  administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase   administrator.LoginAdministratorUseCase
	LogoutAdministratorUseCase  administrator.LogoutAdministratorUseCase

	SendMessageUseCase       messageusecase.SendMessageUseCase
	ListMessagesUseCase      messageusecase.ListMessagesUseCase
	CreateRoomUseCase        roomusecase.CreateRoomUseCase
	GetRoomUseCase           roomusecase.GetRoomUseCase
	GetOrCreateDMRoomUseCase roomusecase.GetOrCreateDMRoomUseCase
	AddUserToRoomUseCase     roomusecase.AddUserToRoomUseCase
	RemoveUserFromRoomUseCase roomusecase.RemoveUserFromRoomUseCase

	PubSub *pubsub.PubSub
}
