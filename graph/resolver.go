package graph

import (
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

var usersMemory = []*model.User{}

var postsMemory = []*model.Post{}

type Resolver struct {
	//SignUpUseCase   user.SignUpUseCase
	//GetUserUseCase  user.GetUserUseCase
	//GetUsersUseCase user.GetUsersUseCase
}
