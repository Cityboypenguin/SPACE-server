package post

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/mention"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// MaxMentionsPerPost は1投稿で有効になるメンションの上限。
// 上限を超えた分は通常テキストとして扱い、通知もしない（メンション爆撃の抑止）。
const MaxMentionsPerPost = 10

// ResolveMentions は本文中の @accountID を実在するユーザーへ解決する。
//
// 除外するもの:
//   - 存在しない / 凍結済みの accountID（GetUsersByAccountIDs 側で除外される）
//   - 自分自身（自己メンションでは通知しない。リンク化もしない）
//   - ブロックしている / されている相手（メンション経由の接触を断つ）
//
// Mention.Text には本文に書かれた通りの表記を入れる。相手が後から accountID を
// 変更しても本文は "@旧ID" のまま残るため、表示側はこの値と突き合わせて着色する。
func ResolveMentions(
	ctx context.Context,
	userRepo repository.UserReader,
	blockerRepo repository.BlockerRepository,
	content string,
	authorID int64,
) ([]*model.Mention, error) {
	accountIDs := mention.ExtractAccountIDs(content, MaxMentionsPerPost)
	if len(accountIDs) == 0 {
		return nil, nil
	}

	users, err := userRepo.GetUsersByAccountIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}

	byAccountID := make(map[string]*model.User, len(users))
	for _, u := range users {
		byAccountID[strings.ToLower(u.AccountID)] = u
	}

	blocked := make(map[int64]struct{})
	if blockerRepo != nil {
		ids, err := blockerRepo.GetBlockedAndBlockerIDs(ctx, authorID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			blocked[id] = struct{}{}
		}
	}

	mentions := make([]*model.Mention, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		u, ok := byAccountID[strings.ToLower(accountID)]
		if !ok || u.ID == authorID {
			continue
		}
		if _, isBlocked := blocked[u.ID]; isBlocked {
			continue
		}
		mentions = append(mentions, &model.Mention{UserID: u.ID, Text: accountID})
	}
	return mentions, nil
}

// NotifyMentions はメンションされた各ユーザーへ通知を送る。
//
// skipUserID には「別の通知を既に送った相手」（返信通知の宛先）を渡す。
// 返信と同時に相手をメンションしても通知が2通にならないようにするため。
// 配信失敗は投稿の成否に影響させない（ログのみ）。
func NotifyMentions(
	ctx context.Context,
	publisher notificationuc.NotificationPublisher,
	mentions []*model.Mention,
	postID int64,
	actorID int64,
	skipUserID *int64,
) {
	if publisher == nil || len(mentions) == 0 {
		return
	}

	targetType := notificationuc.TargetPost
	params := make([]notificationuc.PublishParams, 0, len(mentions))
	for _, m := range mentions {
		if skipUserID != nil && m.UserID == *skipUserID {
			continue
		}
		params = append(params, notificationuc.PublishParams{
			UserID:     m.UserID,
			Type:       notificationuc.TypeMention,
			ActorID:    &actorID,
			TargetType: &targetType,
			TargetID:   &postID,
			Message:    notificationuc.MessageMentionedInPost,
		})
	}
	if len(params) == 0 {
		return
	}

	if err := publisher.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).Msg("failed to publish mention notifications")
	}
}
