package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// MessageReactionRepository はメッセージへのリアクション（絵文字）の保存と集計。
type MessageReactionRepository interface {
	// AddMessageReaction は冪等。同じ人が同じ絵文字をもう一度付けても何も起きない。
	AddMessageReaction(ctx context.Context, messageID, userID int64, emoji string) error
	// RemoveMessageReaction は冪等。付けていない絵文字を外しても何も起きない。
	RemoveMessageReaction(ctx context.Context, messageID, userID int64, emoji string) error
	// ListMessageReactionsByMessageIDs は複数のメッセージのリアクションを1クエリで
	// 集計する（DataLoader 用）。結果の絵文字は付けられた順（先に付いたものが前）。
	// リアクションが1つも無いメッセージは map に入らない。
	ListMessageReactionsByMessageIDs(ctx context.Context, messageIDs []int64, viewerUserID int64) (map[int64][]*model.MessageReaction, error)
}
