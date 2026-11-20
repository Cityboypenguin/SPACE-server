package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommunityInvitationRepository struct{ DB *sql.DB }

const invitationColumns = "id, room_id, sender_id, recipient_id, state, created_at, expires_at"

func scanInvitation(row rowScanner) (*model.CommunityInvitation, error) {
	i := new(model.CommunityInvitation)
	var createdAt, expiresAt int64
	err := row.Scan(&i.ID, &i.RoomID, &i.SenderID, &i.RecipientID, &i.State, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	i.CreatedAt, i.ExpiresAt = time.Unix(createdAt, 0), time.Unix(expiresAt, 0)
	return i, err
}

func (r *CommunityInvitationRepository) Create(ctx context.Context, i *model.CommunityInvitation, digest []byte) error {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		INSERT INTO community_invitations (room_id, sender_id, recipient_id, token_digest, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, i.RoomID, i.SenderID, i.RecipientID, digest, i.CreatedAt.Unix(), i.ExpiresAt.Unix())
	if err != nil {
		return err
	}
	i.ID, err = result.LastInsertId()
	return err
}

func (r *CommunityInvitationRepository) Get(ctx context.Context, id int64, lock bool) (*model.CommunityInvitation, error) {
	query := "SELECT " + invitationColumns + " FROM community_invitations WHERE id = ?"
	if lock {
		query += " FOR UPDATE"
	}
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx, query, id))
}

func (r *CommunityInvitationRepository) GetByToken(ctx context.Context, digest []byte) (*model.CommunityInvitation, error) {
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx,
		"SELECT "+invitationColumns+" FROM community_invitations WHERE token_digest = ?", digest))
}

func (r *CommunityInvitationRepository) Pending(ctx context.Context, roomID, recipientID int64, now time.Time) (*model.CommunityInvitation, error) {
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx,
		"SELECT "+invitationColumns+` FROM community_invitations
		WHERE room_id = ? AND recipient_id = ? AND state = 'pending' AND expires_at > ?
		ORDER BY id DESC LIMIT 1 FOR UPDATE`, roomID, recipientID, now.Unix()))
}

func (r *CommunityInvitationRepository) ListReceived(ctx context.Context, recipientID int64, now time.Time, limit, offset int32) ([]*model.CommunityInvitation, int32, error) {
	const filter = ` FROM community_invitations i
		JOIN users sender ON sender.id = i.sender_id
		WHERE sender.status <> 'deactivated' AND i.recipient_id = ? AND i.state = 'pending' AND i.expires_at > ?
		AND EXISTS (SELECT 1 FROM room_users WHERE room_id = i.room_id AND user_id = i.sender_id)
		AND NOT EXISTS (SELECT 1 FROM room_users WHERE room_id = i.room_id AND user_id = i.recipient_id)
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE
			(b.user_id = i.sender_id AND b.blocked_user_id = i.recipient_id) OR
			(b.user_id = i.recipient_id AND b.blocked_user_id = i.sender_id))`
	db := extractDB(ctx, r.DB)
	var total int32
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*)"+filter, recipientID, now.Unix()).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit == 0 || total == 0 {
		return []*model.CommunityInvitation{}, total, nil
	}
	items, err := r.list(ctx, "SELECT i.id, i.room_id, i.sender_id, i.recipient_id, i.state, i.created_at, i.expires_at"+filter+" ORDER BY i.id DESC LIMIT ? OFFSET ?", recipientID, now.Unix(), limit, offset)
	return items, total, err
}

func (r *CommunityInvitationRepository) ListSent(ctx context.Context, roomID, actorID int64, owner bool, now time.Time, limit, offset int32) ([]*model.CommunityInvitation, error) {
	return r.list(ctx, "SELECT "+invitationColumns+` FROM community_invitations i
		WHERE i.room_id = ? AND (i.sender_id = ? OR ?) AND i.state = 'pending' AND i.expires_at > ?
		AND (i.recipient_id IS NULL OR NOT EXISTS (SELECT 1 FROM room_users WHERE room_id = i.room_id AND user_id = i.recipient_id))
		ORDER BY i.id DESC LIMIT ? OFFSET ?`, roomID, actorID, owner, now.Unix(), limit, offset)
}

func (r *CommunityInvitationRepository) list(ctx context.Context, query string, args ...any) ([]*model.CommunityInvitation, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*model.CommunityInvitation, 0)
	for rows.Next() {
		i, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func (r *CommunityInvitationRepository) RecentCount(ctx context.Context, roomID, senderID int64, since time.Time) (int, error) {
	var count int
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM community_invitations WHERE room_id = ? AND sender_id = ? AND created_at >= ?`, roomID, senderID, since.Unix()).Scan(&count)
	return count, err
}

func (r *CommunityInvitationRepository) RecentlyDeclined(ctx context.Context, roomID, recipientID int64, now time.Time) (bool, error) {
	var exists bool
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM community_invitations
		WHERE room_id = ? AND recipient_id = ? AND state = 'declined' AND expires_at > ?)`,
		roomID, recipientID, now.Unix()).Scan(&exists)
	return exists, err
}

func (r *CommunityInvitationRepository) SetState(ctx context.Context, id int64, state string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, "UPDATE community_invitations SET state = ? WHERE id = ?", state, id)
	return err
}

func (r *CommunityInvitationRepository) AcceptReceived(ctx context.Context, roomID, recipientID int64, now time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE community_invitations SET state = 'accepted'
		WHERE room_id = ? AND recipient_id = ? AND state = 'pending' AND expires_at > ?`, roomID, recipientID, now.Unix())
	return err
}

func (r *CommunityInvitationRepository) JoinStampSettings(ctx context.Context) (model.CommunityJoinStampSettings, error) {
	settings := model.CommunityJoinStampSettings{WelcomeIDs: []string{}, GreetingIDs: []string{}}
	var welcome, greeting []byte
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT welcome_ids, greeting_ids FROM community_join_stamp_settings WHERE id = 1`).Scan(&welcome, &greeting)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(welcome, &settings.WelcomeIDs); err != nil {
		return settings, err
	}
	return settings, json.Unmarshal(greeting, &settings.GreetingIDs)
}

func (r *CommunityInvitationRepository) SaveJoinStampSettings(ctx context.Context, settings model.CommunityJoinStampSettings) error {
	welcome, err := json.Marshal(settings.WelcomeIDs)
	if err != nil {
		return err
	}
	greeting, err := json.Marshal(settings.GreetingIDs)
	if err != nil {
		return err
	}
	_, err = extractDB(ctx, r.DB).ExecContext(ctx, `
		INSERT INTO community_join_stamp_settings (id, welcome_ids, greeting_ids) VALUES (1, ?, ?) AS incoming
		ON DUPLICATE KEY UPDATE welcome_ids = incoming.welcome_ids, greeting_ids = incoming.greeting_ids`, string(welcome), string(greeting))
	return err
}

func (r *CommunityInvitationRepository) WelcomeMessage(ctx context.Context, roomID, userID int64) (*int64, error) {
	var id *int64
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT ru.welcome_message_id FROM room_users ru
		JOIN messages m ON m.id = ru.welcome_message_id AND m.deleted_at IS NULL
		WHERE ru.room_id = ? AND ru.user_id = ? AND NOT ru.greeting_sent`, roomID, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return id, err
}

func (r *CommunityInvitationRepository) SetWelcomeMessage(ctx context.Context, roomID, userID, messageID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		"UPDATE room_users SET welcome_message_id = ? WHERE room_id = ? AND user_id = ?", messageID, roomID, userID)
	return err
}

func (r *CommunityInvitationRepository) MarkGreetingSent(ctx context.Context, roomID, userID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		"UPDATE room_users SET greeting_sent = TRUE WHERE room_id = ? AND user_id = ?", roomID, userID)
	return err
}
