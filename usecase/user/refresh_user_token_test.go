package user

import (
	"context"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type refreshUserRepo struct{ repository.UserRepository }

func (*refreshUserRepo) GetUserAccountByID(context.Context, int64) (*model.UserAccount, error) {
	return &model.UserAccount{User: model.User{ID: 7, Role: "user", Status: model.UserStatusActive}}, nil
}
func (*refreshUserRepo) GetCredentialsVersionByID(context.Context, int64) (int64, error) {
	return 2, nil
}

type consumingTokens struct {
	repository.RevokedTokenRepository
	mu       sync.Mutex
	consumed map[string]bool
}

func (r *consumingTokens) ConsumeToken(_ context.Context, token string, _ int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.consumed[token] {
		return false, nil
	}
	r.consumed[token] = true
	return true, nil
}

func TestRefreshUserTokenIsSingleUse(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	token, err := auth.GenerateUserRefreshToken(7, "user", 2)
	if err != nil {
		t.Fatal(err)
	}
	uc := NewRefreshUserTokenUseCase(&refreshUserRepo{}, &consumingTokens{consumed: make(map[string]bool)})
	const workers = 12
	results := make(chan *RefreshUserTokenResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _ := uc.Execute(context.Background(), token)
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	var successes int
	for result := range results {
		if result != nil {
			successes++
			if result.RefreshToken == token || result.AccessToken == "" {
				t.Fatal("refresh did not produce a distinct token pair")
			}
		}
	}
	if successes != 1 {
		t.Fatalf("parallel refresh succeeded %d times, want exactly one", successes)
	}
}
