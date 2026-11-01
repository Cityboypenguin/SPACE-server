package chat

import (
	"context"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"strings"
	"time"
	"unicode/utf8"
)

type ThreadService struct {
	Messages  repository.MessageReader
	Access    AccessPolicy
	Rooms     repository.RoomRepository
	Threads   repository.ChatThreadRepository
	Tx        repository.TxManager
	Writers   MessageWriters
	Role      roomusecase.GetRoomUserRoleUseCase
	Anonymous anonusecase.GetOrCreateAnonymousIdentityUseCase
	Events    EventPublisher
}

func threadTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 100 {
		return "", apperr.InvalidInput("タイトルは1〜100文字で入力してください")
	}
	return title, nil
}
func (s *ThreadService) Create(ctx context.Context, parentID int64, title string) (*model.ChatThread, error) {
	title, err := threadTitle(title)
	if err != nil {
		return nil, err
	}
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	access, err := s.Access.ensureWriteAccessFor(ctx, claims, parentID)
	if err != nil {
		return nil, err
	}
	parent := access.Room
	if parent.ParentRoomID != nil || (parent.Type != model.RoomTypeCommunity && parent.Type != model.RoomTypeCourse) {
		return nil, apperr.InvalidInput("このルームにはスレッドを作成できません")
	}
	if parent.Type == model.RoomTypeCourse {
		if _, err := s.Anonymous.Execute(ctx, parentID, claims.ID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	room := &model.Room{Name: title, Type: parent.Type, ParentRoomID: &parentID, ThreadCreatorID: &claims.ID, CreatedAt: now, UpdatedAt: now}
	var announcement *model.Message
	err = s.Tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.Rooms.SaveRoom(ctx, room); err != nil {
			return err
		}
		if err := s.Threads.Create(ctx, room.ID, claims.ID); err != nil {
			return err
		}
		announcement, err = model.NewMessage(model.CreateMessageParam{RoomID: parentID, UserID: claims.ID, Content: "スレッドを作成しました", CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		announcement.ThreadRoomID = &room.ID
		return s.Writers.system.SaveMessage(ctx, announcement)
	})
	if err != nil {
		return nil, err
	}
	if s.Events != nil {
		s.Events.MessageSent(ctx, MessageSentEvent{Room: parent, Message: announcement, ActorID: claims.ID, MemberIDs: access.MemberIDs})
	}
	return &model.ChatThread{Room: room}, nil
}
func (s *ThreadService) Get(ctx context.Context, id int64) (*model.ChatThread, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	room, err := s.Access.EnsureReadAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	if room.ParentRoomID == nil {
		return nil, apperr.InvalidInput("スレッドが見つかりません")
	}
	return s.Threads.Get(ctx, id, claims.ID)
}
func (s *ThreadService) List(ctx context.Context, parentID int64, limit, offset int) ([]*model.ChatThread, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	room, err := s.Access.EnsureReadAccess(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if room.ParentRoomID != nil || (room.Type != model.RoomTypeCommunity && room.Type != model.RoomTypeCourse) {
		return nil, apperr.InvalidInput("このルームにはスレッドがありません")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		return nil, apperr.InvalidInput("offset must be nonnegative")
	}
	return s.Threads.List(ctx, parentID, claims.ID, limit, offset)
}
func (s *ThreadService) CanManage(ctx context.Context, room *model.Room) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	if authz.IsAdminRole(claims.Role) || (room.ThreadCreatorID != nil && *room.ThreadCreatorID == claims.ID) {
		return true, nil
	}
	if room.Type == model.RoomTypeCommunity {
		role, err := s.Role.Execute(ctx, room.PolicyRoomID())
		return role == "owner", err
	}
	return false, nil
}
func (s *ThreadService) Update(ctx context.Context, id int64, title *string, closed *bool) (*model.ChatThread, error) {
	thread, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, apperr.InvalidInput("スレッドが見つかりません")
	}
	room := thread.Room
	if _, err = s.Access.EnsureWriteAccess(ctx, room.PolicyRoomID()); err != nil {
		return nil, err
	}
	ok, err := s.CanManage(ctx, room)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("スレッドを変更する権限がありません")
	}
	name := room.Name
	if title != nil {
		name, err = threadTitle(*title)
		if err != nil {
			return nil, err
		}
	}
	isClosed := room.ThreadClosedAt != nil
	if closed != nil {
		isClosed = *closed
	}

	var announcement *model.Message
	var parentRoom *model.Room
	// Refresh the parent announcement using the usual message subscription channel.
	if s.Messages != nil && s.Events != nil {
		announcementID, err := s.Threads.AnnouncementID(ctx, id)
		if err != nil {
			return nil, err
		}
		if announcementID != 0 {
			msg, err := s.Messages.GetMessageByID(ctx, announcementID)
			if err != nil {
				return nil, err
			}
			parent, err := s.Rooms.GetRoomByID(ctx, room.PolicyRoomID())
			if err != nil {
				return nil, err
			}
			announcement, parentRoom = msg, parent
		}
	}
	if err = s.Tx.RunInTx(ctx, func(ctx context.Context) error { return s.Threads.Update(ctx, id, name, isClosed) }); err != nil {
		return nil, err
	}

	if announcement != nil && parentRoom != nil {
		s.Events.MessageUpdated(ctx, MessageUpdatedEvent{Room: parentRoom, Message: announcement, ActorID: announcement.UserID})
	}

	return s.Get(ctx, id)
}
