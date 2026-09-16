package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/mention"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// MentionsSupported は、そのルームでメンションが成立するかを返す。
//
// コミュニティだけが対象。授業内チャットは匿名なので実名メンションを許すと匿名性が
// 崩れ、DM は相手が1人なので意味がない。
// 「どのルームでメンションできるか」の判断がリゾルバと使用側に散らばらないよう、
// 送信時の検証（ResolveMentionsInteractor）とサジェスト（mentionCandidates リゾルバ）の
// 両方がこの1つを参照する。
func MentionsSupported(room *model.Room) bool {
	return room != nil && room.Type == model.RoomTypeCommunity
}

// MaxMentionsPerMessage は1メッセージで有効になるメンションの上限。
// 上限を超えた分は通常テキストとして扱い、通知もしない（メンション爆撃の抑止）。
const MaxMentionsPerMessage = 10

// ResolveMentionsUseCase はクライアントが送ってきたメンション先の候補を検証し、
// 保存してよいメンションだけに絞り込む。
//
// コミュニティのメンションは "@表示名" で、表示名には空白も記号も入りうるため
// 本文からは終端を決められない。そのため「サジェストから選んだものだけが成立する」
// 仕様にしてあり、クライアントが選択したユーザーIDを送ってくる。
// ただしクライアントの申告をそのまま信じるわけにはいかないので、ここで
// 「本当にそのルームのメンバーか」「本文に @表示名 が実際に残っているか」を検証する。
type ResolveMentionsUseCase interface {
	Execute(ctx context.Context, roomID, authorID int64, content string, mentionUserIDs []int64) ([]*model.Mention, error)
}

var _ ResolveMentionsUseCase = &ResolveMentionsInteractor{}

type ResolveMentionsInteractor struct {
	userRepo     repository.UserRepository
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomUserRepository
	blockerRepo  repository.BlockerRepository
}

func NewResolveMentionsUseCase(
	userRepo repository.UserRepository,
	roomRepo repository.RoomRepository,
	roomUserRepo repository.RoomUserRepository,
	blockerRepo repository.BlockerRepository,
) ResolveMentionsUseCase {
	return &ResolveMentionsInteractor{
		userRepo:     userRepo,
		roomRepo:     roomRepo,
		roomUserRepo: roomUserRepo,
		blockerRepo:  blockerRepo,
	}
}

func (uc *ResolveMentionsInteractor) Execute(ctx context.Context, roomID, authorID int64, content string, mentionUserIDs []int64) ([]*model.Mention, error) {
	// 重複を除き、上限まで切り詰める。
	seen := make(map[int64]struct{}, len(mentionUserIDs))
	ids := make([]int64, 0, len(mentionUserIDs))
	for _, id := range mentionUserIDs {
		if id == authorID {
			continue // 自己メンションは成立させない
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) >= MaxMentionsPerMessage {
			break
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// メンションが成立しないルーム（授業内チャット・DM）では、クライアントが
	// mentionUserIDs を送ってきても黙って捨てる。
	room, err := uc.roomRepo.GetRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !MentionsSupported(room) {
		return nil, nil
	}

	memberIDs, err := uc.roomUserRepo.GetUserIDsByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	members := make(map[int64]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		members[id] = struct{}{}
	}

	blockedIDs, err := uc.blockerRepo.GetBlockedAndBlockerIDs(ctx, authorID)
	if err != nil {
		return nil, err
	}
	blocked := make(map[int64]struct{}, len(blockedIDs))
	for _, id := range blockedIDs {
		blocked[id] = struct{}{}
	}

	users, err := uc.userRepo.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*model.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	mentions := make([]*model.Mention, 0, len(ids))
	for _, id := range ids {
		u, ok := byID[id]
		if !ok || u.Status != model.UserStatusActive {
			continue
		}
		if _, isMember := members[id]; !isMember {
			continue
		}
		if _, isBlocked := blocked[id]; isBlocked {
			continue
		}
		// サジェストで挿入したあとに本文側を書き換えたケースを弾く。
		if !mention.ContainsText(content, u.Name) {
			continue
		}
		mentions = append(mentions, &model.Mention{UserID: u.ID, Text: u.Name})
	}
	return mentions, nil
}
