package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// MarkAsRead は roomID を呼び出し元の既読にする。
//
// 既読位置の置き場はルーム種別で分かれる。授業内チャットは room_users を使わない
// ので course_room_reads に、それ以外は room_users に持つ。
//
// ここで EnsureReadAccess を使わないのは意図的で、既読は「読めるか」ではなく
// 「自分の既読位置を持てるか」の話。管理者が非メンバーのコミュニティを覗いたときに
// room_users へ既読位置を書こうとしても行が無いので、統合前と同じく membership を
// 要求する。
func (s *service) MarkAsRead(ctx context.Context, roomID int64) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}

	room, err := s.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to get room")
	}

	var memberIDs []int64
	if room.Type == model.RoomTypeCourse {
		if err := s.deps.MarkCourseRoomAsRead.Execute(ctx, roomID, claims.ID); err != nil {
			return err
		}
	} else {
		memberIDs, err = s.deps.GetRoomMemberIDs.Execute(ctx, roomID)
		if err != nil {
			return fmt.Errorf("failed to verify room membership")
		}
		if !containsInt64(memberIDs, claims.ID) {
			return errors.New("forbidden: not a member of this room")
		}
		if err := s.deps.MarkRoomAsRead.Execute(ctx, roomID, claims.ID); err != nil {
			return err
		}
	}

	s.deps.Events.RoomMarkedAsRead(ctx, RoomMarkedAsReadEvent{
		Room:      room,
		ActorID:   claims.ID,
		MemberIDs: memberIDs,
	})
	return nil
}

// GetReadStatus は呼び出し元自身の既読位置と未読件数を返す。
// 授業内チャットは匿名なので、他人の既読位置（PartnerLastReadAt）は常に nil。
func (s *service) GetReadStatus(ctx context.Context, roomID int64) (*roomusecase.RoomReadStatus, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.Type == model.RoomTypeCourse {
		return s.deps.GetCourseRoomReadStatus.Execute(ctx, roomID, claims.ID)
	}
	return s.deps.GetRoomReadStatus.Execute(ctx, roomID, claims.ID)
}
