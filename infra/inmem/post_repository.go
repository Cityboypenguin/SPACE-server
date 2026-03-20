package inmem

import (
	"context"
	"sort"
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

func (r *InmemPostRepository) GetPost(ctx context.Context, id int64) (*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	p, ok := r.posts[id]
	if !ok {
		return nil, nil
	}
	copy := *p
	return &copy, nil
}

func (r *InmemPostRepository) GetPosts(ctx context.Context) ([]*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	postList := make([]*model.Post, 0, len(r.posts))
	for _, p := range r.posts {
		copy := *p
		postList = append(postList, &copy)
	}
	sort.Slice(postList, func(i, j int) bool {
		return postList[i].ID < postList[j].ID
	})
	return postList, nil
}

func (r *InmemPostRepository) create(_ context.Context, p *model.Post) error {
	p.ID = r.nextID
	r.nextID++
	stored := *p
	r.posts[p.ID] = &stored
	return nil
}

func (r *InmemPostRepository) update(_ context.Context, p *model.Post) error {
	stored := *p
	r.posts[p.ID] = &stored
	return nil
}
