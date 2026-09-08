package message

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeMessageRepo is a minimal in-memory MessageRepository shared by the
// usecase/message tests. It embeds repository.MessageRepository so any method
// not overridden here panics if a test unexpectedly exercises it, and is safe
// for concurrent use by the goroutine-based tests (concurrency_test.go).
type fakeMessageRepo struct {
	repository.MessageRepository

	mu       sync.Mutex
	messages map[int64]*model.Message
	nextID   int64

	// beforeSave, when set, runs while the lock is held but before the message
	// is assigned an ID and stored — used to widen race windows in concurrency tests.
	beforeSave func()
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{messages: make(map[int64]*model.Message)}
}

func (f *fakeMessageRepo) SaveMessage(_ context.Context, m *model.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beforeSave != nil {
		f.beforeSave()
	}
	f.nextID++
	m.ID = f.nextID
	cp := *m
	f.messages[m.ID] = &cp
	return nil
}

func (f *fakeMessageRepo) GetMessageByID(_ context.Context, id int64) (*model.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok || m.IsDeleted() {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}

// getMessageByIDIncludingDeleted bypasses the deleted_at filter, mirroring what
// an admin-only lookup would see; used to assert soft-delete fields directly.
func (f *fakeMessageRepo) getMessageByIDIncludingDeleted(id int64) *model.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return nil
	}
	cp := *m
	return &cp
}

func (f *fakeMessageRepo) SoftDeleteMessage(_ context.Context, id int64, deletedBy int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok || m.IsDeleted() {
		return false, nil
	}
	now := time.Now()
	m.DeletedAt = &now
	by := deletedBy
	m.DeletedBy = &by
	return true, nil
}

func (f *fakeMessageRepo) UpdateMessage(_ context.Context, m *model.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.messages[m.ID]
	if !ok || existing.IsDeleted() {
		return nil
	}
	cp := *m
	f.messages[m.ID] = &cp
	return nil
}

func (f *fakeMessageRepo) ListMessagesByRoomID(_ context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ids := make([]int64, 0, len(f.messages))
	for id := range f.messages {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var result []*model.Message
	for _, id := range ids {
		m := f.messages[id]
		if m.RoomID != roomID || m.IsDeleted() {
			continue
		}
		if beforeID != nil && id >= *beforeID {
			continue
		}
		if afterID != nil && id <= *afterID {
			continue
		}
		if afterTime != nil && !m.CreatedAt.After(*afterTime) {
			continue
		}
		cp := *m
		result = append(result, &cp)
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, false, false, nil
}

// fakeMediaRepo is unused by any current test (no test sends media), but is
// provided so SendMessageInteractor can always be constructed with a non-nil
// dependency, matching NewSendMessageUseCase's signature.
type fakeMediaRepo struct {
	repository.MediaRepository
}

// fakeTxManager runs the given function directly without a real transaction,
// which is sufficient since fakeMessageRepo/fakeMediaRepo have no rollback semantics.
type fakeTxManager struct{}

func (fakeTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
