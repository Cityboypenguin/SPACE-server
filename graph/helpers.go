// 共通のヘルパーは役割ごとのファイルに分かれている。
// auth_helpers / id_helpers / pagination_helpers / selection_helpers /
// subscription_helpers / presenter_helpers / upload_helpers を参照。
//
// このファイルに残っているのは、コミュニティのメンバー更新と、
// チャット表示まわりの取得失敗ログの名前。
package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	"github.com/rs/zerolog"
)

// updateCommunityMembers applies the membership change atomically and only then
// emits the corresponding notifications. Notification delivery is best effort,
// matching the behavior of the former single-member mutations.
func (r *Resolver) updateCommunityMembers(ctx context.Context, communityID int64, updates []communityusecase.MemberUpdate) error {
	claims, err := requireAuth(ctx)
	if err != nil {
		return err
	}
	roomID, err := r.UpdateCommunityMembersUseCase.Execute(ctx, communityID, updates)
	if err != nil {
		return err
	}

	// 除名された利用者の購読をその場で終わらせる。役割の変更（昇格・降格）は
	// 閲覧できるかどうかを変えないので合図に含めない。
	r.publishRoomAccessChanged(roomID, kickedUserIDs(updates))

	targetType := notificationuc.TargetCommunity
	params := make([]notificationuc.PublishParams, 0, len(updates))
	for _, update := range updates {
		param := notificationuc.PublishParams{
			UserID:     update.UserID,
			TargetType: &targetType,
			TargetID:   &communityID,
		}
		switch update.Action {
		case communityusecase.MemberActionPromote:
			param.Type = notificationuc.TypeCommunityRole
			param.Message = notificationuc.MessagePromotedToCommunityOwner
		case communityusecase.MemberActionDemote:
			param.Type = notificationuc.TypeCommunityRole
			param.Message = notificationuc.MessageDemotedFromCommunityOwner
		case communityusecase.MemberActionKick:
			param.Type = notificationuc.TypeCommunityKick
			param.ActorID = &claims.ID
			param.Message = notificationuc.MessageKickedFromCommunity
		default:
			continue
		}
		params = append(params, param)
	}

	if err := r.NotificationPublisher.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).
			Int64("community_id", communityID).
			Msg("failed to publish community member update notifications")
	}
	return nil
}

// kickedUserIDs は更新指定のうち除名されたものの利用者IDを返す。
func kickedUserIDs(updates []communityusecase.MemberUpdate) []int64 {
	var ids []int64
	for _, u := range updates {
		if u.Action == communityusecase.MemberActionKick {
			ids = append(ids, u.UserID)
		}
	}
	return ids
}

// チャット表示まわりで「取れなくても画面は返す」取得の失敗ログに使う lookup 名。
// 配信側の chatDelivery* と同じく、grep する側が経路で絞れるよう1箇所に集める。
const (
	chatLookupCourseRoom         = "course_room_lookup"
	chatLookupAnonymousLabel     = "anonymous_label_lookup"
	chatLookupQuestionRoom       = "question_room_lookup"
	chatLookupPollAuthorRole     = "poll_author_role_lookup"
	chatLookupBlockRelation      = "block_relation_lookup"
	chatLookupBlockedUserIDs     = "blocked_user_ids_lookup"
	chatLookupLastMessages       = "last_messages_lookup"
	chatLookupReadStatus         = "read_status_lookup"
	chatLookupReadStatusBatch    = "read_status_batch_lookup"
	chatLookupNotificationTarget = "notification_target_lookup"
)

// logChatLookup はリゾルバ側のベストエフォートな取得失敗を、必ず同じ形で残す。
//
// 配信側の logChatDelivery（graph/chat_events.go）と対になる口。どちらも
// 「失敗しても処理は続ける」場所なので、黙って落とすと DB 障害が
// 「未読0」「実名表示」といった正常に見える表示へ化けて誰も気づけない。
// 呼び出し側は room_id / message_id など後から追える情報を足してから Msg すること。
func logChatLookup(err error, lookup string) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_resolver").
		Str("lookup", lookup)
}
