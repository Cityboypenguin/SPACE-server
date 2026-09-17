package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// EnsureReadAccess は roomID を閲覧してよいかを判定する。
//
// 判定規則:
//   - 授業内チャット: 認証済みなら誰でも閲覧可（F-04 全授業公開。投稿者は匿名表示）。
//   - それ以外: room_users の membership が要る。
//   - 管理者: DM 以外なら非メンバーでも閲覧可。
//
// 管理者の扱いについて。統合前は messages クエリだけが isAdminRole を見ていて、
// messageAdded などのサブスクリプションは見ていなかった。同じ部屋なのに
// 「一覧は出るが購読は繋がらない」という食い違いが出るので、緩い側（管理者は
// 閲覧可）へ揃えた。ただし DM は例外で、統合前に管理者向けの DM 閲覧機能が
// 無かった以上ここで広げる理由が無く、通報対応は通報時のスナップショットで
// 足りるため、管理者でも membership を要求する（統合前の messages クエリより
// 厳しくなるのはこの1点だけ）。
func (s *service) EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		// GetRoomUseCase は存在しないルームに "room not found: <id>" を返す。
		// クライアントの not-found 判定がそのまま効くよう、包まずに返す。
		return nil, err
	}
	if room.Type == model.RoomTypeCourse {
		return room, nil
	}

	memberIDs, err := s.deps.GetRoomMemberIDs.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership")
	}
	if containsInt64(memberIDs, claims.ID) {
		return room, nil
	}
	if room.Type != model.RoomTypeDM && authz.IsAdminRole(claims.Role) {
		return room, nil
	}
	return nil, errors.New("forbidden: not a member of this room")
}

// EnsureWriteAccess は roomID へ書き込んでよいかを判定する。
// 判定の中身は ensureWriteAccess と同じで、こちらはルームだけを返す薄い口。
func (s *service) EnsureWriteAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	access, err := s.ensureWriteAccess(ctx, claims, roomID)
	if err != nil {
		return nil, err
	}
	return access.Room, nil
}

// writeAccess は書き込み判定の結果。
type writeAccess struct {
	Room *model.Room
	// MemberIDs は非授業ルームのメンバー。判定のついでに引いた結果を、送信後の
	// DM 通知でもう一度引き直さずに済むよう持ち回す。授業ルームでは nil。
	MemberIDs []int64
}

// ensureWriteAccess は書き込み権限の唯一の判定。
//
//   - 授業内チャット: room_users を使わず、現在の学期と一致するか（アーカイブ
//     されていないか）と時間割に登録済みかを CheckRoomWritableUseCase が見る。
//   - それ以外: membership が必須。2人のルーム（DM）はブロック関係があれば拒否。
func (s *service) ensureWriteAccess(ctx context.Context, claims *auth.Claims, roomID int64) (*writeAccess, error) {
	room, err := s.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}

	if room.Type == model.RoomTypeCourse {
		if err := s.deps.CheckRoomWritable.Execute(ctx, roomID); err != nil {
			return nil, err
		}
		return &writeAccess{Room: room}, nil
	}

	memberIDs, err := s.deps.GetRoomMemberIDs.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership")
	}
	if !containsInt64(memberIDs, claims.ID) {
		return nil, errors.New("forbidden: not a member of this room")
	}

	if len(memberIDs) == 2 {
		var partnerID int64
		for _, id := range memberIDs {
			if id != claims.ID {
				partnerID = id
				break
			}
		}

		isBlocked, err := s.deps.CheckBlockRelation.Execute(ctx, claims.ID, partnerID)
		if err != nil {
			return nil, fmt.Errorf("failed to check block status")
		}
		if isBlocked {
			return nil, errors.New("ブロック設定によりメッセージを送信できません")
		}
	}

	return &writeAccess{Room: room, MemberIDs: memberIDs}, nil
}

// ensureAnonymousIdentity は授業ルームへの書き込み時に匿名ID（匿名NNN）を確定させる。
//
// 以前は表示時（messageResolver.User など）に採番していたため、(a) 読むだけの
// クエリが DB に行を作る副作用を持ち、(b) 番号が「そのルームで初めて投稿した順」
// ではなく「初めて誰かの画面に出た順」になりえた。投稿時に採番すれば番号は
// 投稿順に固定され、表示側は読み取りだけで済む。
//
// 保存トランザクションの外で呼んでいるのは、採番が MySQL の名前付きロック
// (GET_LOCK) を使う別接続の処理で、トランザクションに参加しないため。保存が
// 失敗しても残るのは「まだ投稿していない人の番号」だけで、その人が次に投稿した
// ときに同じ行が再利用されるので実害はない。
func (s *service) ensureAnonymousIdentity(ctx context.Context, room *model.Room, userID int64) error {
	if room == nil || room.Type != model.RoomTypeCourse {
		return nil
	}
	_, err := s.deps.GetOrCreateAnonymousIdentity.Execute(ctx, room.ID, userID)
	return err
}
