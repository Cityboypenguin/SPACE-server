package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

// CreateAccount
func (r *mutationResolver) CreateAccount(ctx context.Context, name string, email string) (*model.User, error) {
	return r.UserUsecase.CreateUser(ctx, name, email)
}

// UpdateName
func (r *mutationResolver) UpdateName(ctx context.Context, id string, newName string) (*model.User, error) {
	return r.UserUsecase.UpdateName(ctx, id, newName)
}

// UpdateEmail
func (r *mutationResolver) UpdateEmail(ctx context.Context, id string, newEmail string) (*model.User, error) {
	return r.UserUsecase.UpdateEmail(ctx, id, newEmail)
}

// DeleteAccount
func (r *mutationResolver) DeleteAccount(ctx context.Context, id string) (bool, error) {
	return r.UserUsecase.DeleteAccount(ctx, id)
}

// CreatePost
func (r *mutationResolver) CreatePost(ctx context.Context, content string, userId string) (*model.Post, error) {
	return r.PostUsecase.CreatePost(ctx, content, userId)
}

// Users
func (r *queryResolver) Users(ctx context.Context) ([]*model.User, error) {
	return r.UserUsecase.GetUsers(ctx)
}

// User
func (r *queryResolver) User(ctx context.Context, id string) (*model.User, error) {
	return r.UserUsecase.GetUser(ctx, id)
}

// Posts
func (r *queryResolver) Posts(ctx context.Context) ([]*model.Post, error) {
	return r.PostUsecase.GetPosts(ctx)
}

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
