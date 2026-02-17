package repository

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/domain"
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type userRepo struct {
	usersMemory []*model.User // グローバル変数ではなく、ここで管理！
}

func NewUserRepo() domain.UserRepository {
	return &userRepo{usersMemory: []*model.User{}}
}

// 作成
func (r *userRepo) Create(ctx context.Context, user *model.User) (*model.User, error) {
	r.usersMemory = append(r.usersMemory, user)
	return user, nil
}

// 全取得
func (r *userRepo) GetAll(ctx context.Context) ([]*model.User, error) {
	return r.usersMemory, nil
}

// ID検索
func (r *userRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	for _, u := range r.usersMemory {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, fmt.Errorf("ユーザーが見つかりません: %s", id)
}

// 更新 (名前やメールの変更はUsecaseで値を書き換えてからこれを呼ぶ想定)
func (r *userRepo) Update(ctx context.Context, user *model.User) (*model.User, error) {
	for i, u := range r.usersMemory {
		if u.ID == user.ID {
			r.usersMemory[i] = user
			return user, nil
		}
	}
	return nil, fmt.Errorf("更新対象が見つかりません")
}

// 削除
func (r *userRepo) Delete(ctx context.Context, id string) (bool, error) {
	newUsersList := []*model.User{}
	found := false
	for _, u := range r.usersMemory {
		if u.ID == id {
			found = true
			continue
		}
		newUsersList = append(newUsersList, u)
	}
	if !found {
		return false, fmt.Errorf("削除対象が見つかりません")
	}
	r.usersMemory = newUsersList
	return true, nil
}
