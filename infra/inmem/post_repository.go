package inmem

import (
	"context"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.PostRepository = &InmemPostRepository{}

type InmemPostRepository struct {
	posts  map[int64]*model.Post
	nextID int64
	mtx    sync.Mutex
}

func NewInmemPostRepository() *InmemPostRepository {
	return &InmemPostRepository{
		posts:  make(map[int64]*model.Post),
		nextID: 1,
	}
}

func (r *InmemPostRepository) SavePost(ctx context.Context, post *model.Post) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if post.ID == 0 {
		return r.create(ctx, post)
	}
	return r.update(ctx, post)
}

func (r *InmemPostRepository) GetPosts(ctx context.Context) ([]*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	var postList []*model.Post
	for _, p := range r.posts {
		postList = append(postList, p)
	}
	return postList, nil
}

func (r *InmemPostRepository) create(_ context.Context, p *model.Post) error {
	p.ID = r.nextID
	r.nextID++
	r.posts[p.ID] = p
	return nil
}

func (r *InmemPostRepository) update(_ context.Context, p *model.Post) error {
	r.posts[p.ID] = p
	return nil
}
