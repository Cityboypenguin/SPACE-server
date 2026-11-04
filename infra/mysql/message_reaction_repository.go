package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.MessageReactionRepository = &MySQLMessageReactionRepository{}

type MySQLMessageReactionRepository struct {
	DB *sql.DB
}

func NewMySQLMessageReactionRepository(db *sql.DB) *MySQLMessageReactionRepository {
	return &MySQLMessageReactionRepository{DB: db}
}

func (r *MySQLMessageReactionRepository) AddMessageReaction(ctx context.Context, messageID, userID int64, emoji string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO message_reactions (message_id, user_id, emoji, created_at) VALUES (?, ?, ?, ?)`,
		messageID, userID, emoji, time.Now().Unix(),
	)
	return err
}

func (r *MySQLMessageReactionRepository) RemoveMessageReaction(ctx context.Context, messageID, userID int64, emoji string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`DELETE FROM message_reactions WHERE message_id = ? AND user_id = ? AND emoji = ?`,
		messageID, userID, emoji,
	)
	return err
}

func (r *MySQLMessageReactionRepository) ListMessageReactionsByMessageIDs(ctx context.Context, messageIDs []int64, viewerUserID int64) (map[int64][]*model.MessageReaction, error) {
	result := make(map[int64][]*model.MessageReaction, len(messageIDs))
	if len(messageIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(messageIDs)), ",")
	query := fmt.Sprintf(`
		SELECT message_id, emoji,
		       COUNT(*) AS reaction_count,
		       SUM(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS my_reaction_count
		FROM message_reactions
		WHERE message_id IN (%s)
		GROUP BY message_id, emoji
		ORDER BY message_id ASC, MIN(id) ASC
	`, placeholders)

	args := make([]any, 0, len(messageIDs)+1)
	args = append(args, viewerUserID)
	for _, id := range messageIDs {
		args = append(args, id)
	}

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageID int64
			emoji     string
			count     int
			mine      int
		)
		if err := rows.Scan(&messageID, &emoji, &count, &mine); err != nil {
			return nil, err
		}
		result[messageID] = append(result[messageID], &model.MessageReaction{
			Emoji:       emoji,
			Count:       count,
			ReactedByMe: mine > 0,
		})
	}
	return result, rows.Err()
}
