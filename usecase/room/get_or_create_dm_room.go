package room

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetOrCreateDMRoomUseCase interface {
	Execute(ctx context.Context, userID2 int64) (*model.Room, error)
}

var _ GetOrCreateDMRoomUseCase = &GetOrCreateDMRoomInteractor{}

// dmPartnerReader は DM を始める相手を引く口。退会手続き中・削除済みの人は nil で返る。
type dmPartnerReader interface {
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
}

// ErrDMPartnerUnavailable は、退会した（または存在しない）相手と DM を始めようとしたとき。
var ErrDMPartnerUnavailable = errors.New("相手が退会しているため、メッセージを送信できません")

type GetOrCreateDMRoomInteractor struct {
	roomUserRepo repository.DMRoomRepository
	users        dmPartnerReader
}

func NewGetOrCreateDMRoomUseCase(roomUserRepo repository.DMRoomRepository, users dmPartnerReader) GetOrCreateDMRoomUseCase {
	return &GetOrCreateDMRoomInteractor{
		roomUserRepo: roomUserRepo,
		users:        users,
	}
}

func (uc *GetOrCreateDMRoomInteractor) Execute(ctx context.Context, userID2 int64) (*model.Room, error) {
	userID1, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	partner, err := uc.users.GetUserByID(ctx, userID2)
	if err != nil {
		return nil, err
	}
	if partner == nil {
		return nil, ErrDMPartnerUnavailable
	}
	return uc.roomUserRepo.FindOrCreateDMRoom(ctx, userID1, userID2)
}
