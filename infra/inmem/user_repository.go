package inmem

import (
	"context"
	"sort"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.UserRepository = &InmemUserRepository{}

type InmemUserRepository struct {
	users  map[int64]*model.User
	nextID int64
	mtx    sync.Mutex
}

func NewInmemUserRepository() *InmemUserRepository {
	return &InmemUserRepository{
		users:  make(map[int64]*model.User),
		nextID: 1,
	}
}

func (r *InmemUserRepository) SaveUser(ctx context.Context, user *model.User) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if user.ID == 0 {
		return r.create(ctx, user)
	}
	return r.update(ctx, user)
}

func (r *InmemUserRepository) GetUser(ctx context.Context, id int64) (*model.User, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	u, ok := r.users[id]
	if !ok {
		return nil, nil
	}
	copy := *u
	return &copy, nil
}

func (r *InmemUserRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	result := make([]*model.User, 0, len(ids))
	for _, id := range ids {
		if u, ok := r.users[id]; ok {
			copy := *u
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (r *InmemUserRepository) GetUsers(ctx context.Context) ([]*model.User, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	userList := make([]*model.User, 0, len(r.users))
	for _, u := range r.users {
		copy := *u
		userList = append(userList, &copy)
	}
	sort.Slice(userList, func(i, j int) bool {
		return userList[i].ID < userList[j].ID
	})
	return userList, nil
}

func (r *InmemUserRepository) create(_ context.Context, u *model.User) error {
	u.ID = r.nextID
	r.nextID++
	stored := *u
	r.users[u.ID] = &stored
	return nil
}

func (r *InmemUserRepository) update(_ context.Context, u *model.User) error {
	stored := *u
	r.users[u.ID] = &stored
	return nil
}
