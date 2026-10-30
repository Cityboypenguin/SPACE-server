package room

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type dmRoomRepo struct {
	repository.DMRoomRepository
	created int
}

func (r *dmRoomRepo) FindOrCreateDMRoom(context.Context, int64, int64) (*model.Room, error) {
	r.created++
	return &model.Room{ID: 1, Type: model.RoomTypeDM}, nil
}

// dmUsers は表示用の利用者取得の代役。active に無い人は退会済みとして nil を返す。
type dmUsers struct{ active map[int64]bool }

func (u dmUsers) GetUserByID(_ context.Context, id int64) (*model.User, error) {
	if !u.active[id] {
		return nil, nil
	}
	return &model.User{ID: id, Status: model.UserStatusActive}, nil
}

func TestGetOrCreateDMRoomRejectsWithdrawnPartner(t *testing.T) {
	repo := &dmRoomRepo{}
	uc := NewGetOrCreateDMRoomUseCase(repo, dmUsers{active: map[int64]bool{1: true}})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})

	if _, err := uc.Execute(ctx, 2); !errors.Is(err, ErrDMPartnerUnavailable) {
		t.Fatalf("err = %v, want ErrDMPartnerUnavailable", err)
	}
	if repo.created != 0 {
		t.Fatal("a DM room was opened with a withdrawn user")
	}
}

func TestGetOrCreateDMRoomWithActivePartner(t *testing.T) {
	repo := &dmRoomRepo{}
	uc := NewGetOrCreateDMRoomUseCase(repo, dmUsers{active: map[int64]bool{1: true, 2: true}})
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})

	if _, err := uc.Execute(ctx, 2); err != nil || repo.created != 1 {
		t.Fatalf("err=%v created=%d, want the DM room", err, repo.created)
	}
}
