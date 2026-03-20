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

func (r *InmemPostRepository) GetPost(ctx context.Context, id int64) (*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	post, exists := r.posts[id]
	if !exists {
		return nil, repository.ErrPostNotFound
	}
	return post, nil
}

func (r *InmemPostRepository) GetPostsByAuthorID(ctx context.Context, authorID int64) ([]*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	var posts []*model.Post
	for _, post := range r.posts {
		if post.AuthorID == authorID {
			posts = append(posts, post)
		}
	}
	return posts, nil
}

func (r *InmemPostRepository) GetAllPosts(ctx context.Context) ([]*model.Post, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	var posts []*model.Post
	for _, post := range r.posts {
		posts = append(posts, post)
	}
	return posts, nil
}

func (r *InmemPostRepository) UpdatePost(ctx context.Context, post *model.Post) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	_, exists := r.posts[post.ID]
	if !exists {
		return repository.ErrPostNotFound
	}
	r.posts[post.ID] = post
	return nil
}

func (r *InmemPostRepository) create(ctx context.Context, post *model.Post) error {
	post.ID = r.nextID
	r.nextID++
	r.posts[post.ID] = post
	return nil
}

func (r *InmemPostRepository) update(ctx context.Context, post *model.Post) error {
	_, exists := r.posts[post.ID]
	if !exists {
		return repository.ErrPostNotFound
	}
	r.posts[post.ID] = post
	return nil
}