package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// repository.MessageMentionStore の実装。

// CreateMessageMentions はメッセージに紐づくメンションを一括登録する。
// mention_text は表示名そのものなので、messages.content と同じ鍵で暗号化して保存する
// （どのメッセージで誰の名前が呼ばれたかを平文で残さないため）。
func (r *MySQLMessageRepository) CreateMessageMentions(ctx context.Context, messageID int64, mentions []*model.Mention) error {
	if len(mentions) == 0 {
		return nil
	}

	execer := extractDB(ctx, r.DB)
	now := time.Now().Unix()

	var sb strings.Builder
	sb.WriteString("INSERT IGNORE INTO message_mentions (message_id, mentioned_user_id, mention_text, created_at) VALUES ")
	args := make([]interface{}, 0, len(mentions)*4)
	for i, m := range mentions {
		if i > 0 {
			sb.WriteString(", ")
		}
		encrypted, err := r.encryptContent(m.Text)
		if err != nil {
			return err
		}
		sb.WriteString("(?, ?, ?, ?)")
		args = append(args, messageID, m.UserID, encrypted, now)
	}

	_, err := execer.ExecContext(ctx, sb.String(), args...)
	return err
}

// DeleteMessageMentionsByMessageID はメッセージに紐づく全メンションを削除する（編集時の再同期用）。
func (r *MySQLMessageRepository) DeleteMessageMentionsByMessageID(ctx context.Context, messageID int64) error {
	execer := extractDB(ctx, r.DB)
	_, err := execer.ExecContext(ctx, "DELETE FROM message_mentions WHERE message_id = ?", messageID)
	return err
}

// ListMentionsByMessageIDs はメッセージIDごとのメンション一覧を返す。
// メッセージ一覧での N+1 を避けるため DataLoader から1クエリでまとめて呼ばれる。
func (r *MySQLMessageRepository) ListMentionsByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error) {
	result := make(map[int64][]*model.Mention, len(messageIDs))
	if len(messageIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(messageIDs)), ",")
	query := fmt.Sprintf(`
		SELECT message_id, mentioned_user_id, mention_text
		FROM message_mentions
		WHERE message_id IN (%s)
		ORDER BY id ASC
	`, placeholders)

	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var messageID int64
		var m model.Mention
		if err := rows.Scan(&messageID, &m.UserID, &m.Text); err != nil {
			return nil, err
		}
		text, err := r.cipher.Decrypt(m.Text)
		if err != nil {
			return nil, fmt.Errorf("decrypt mention text: %w", err)
		}
		m.Text = text
		result[messageID] = append(result[messageID], &m)
	}
	return result, rows.Err()
}
