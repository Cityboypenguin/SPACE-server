package inmem

import (
	"context"

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

func (r *InmemUserRepository) create(_ context.Context, u *model.User) error {
	u.ID = r.nextID
	r.nextID++
	r.users[u.ID] = u
	return nil
}

func (r *InmemUserRepository) update(_ context.Context, u *model.User) error {
	r.users[u.ID] = u
	return nil
}
