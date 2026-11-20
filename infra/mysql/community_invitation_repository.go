package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommunityInvitationRepository struct{ DB *sql.DB }

const invitationColumns = "i.id, i.room_id, i.sender_id, i.recipient_id, i.state, i.message, i.use_count, i.created_at, i.expires_at, i.responded_at"

func scanInvitation(row rowScanner) (*model.CommunityInvitation, error) {
	i := new(model.CommunityInvitation)
	var createdAt, expiresAt int64
	var respondedAt sql.NullInt64
	err := row.Scan(&i.ID, &i.RoomID, &i.SenderID, &i.RecipientID, &i.State, &i.Message, &i.UseCount,
		&createdAt, &expiresAt, &respondedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	i.CreatedAt, i.ExpiresAt = time.Unix(createdAt, 0), time.Unix(expiresAt, 0)
	if respondedAt.Valid {
		t := time.Unix(respondedAt.Int64, 0)
		i.RespondedAt = &t
	}
	return i, err
}

// activeInvitation は「まだ使える招待」の条件（i は community_invitations の別名）。
const activeInvitation = `i.state = 'pending' AND i.expires_at > ?`

func (r *CommunityInvitationRepository) Create(ctx context.Context, i *model.CommunityInvitation, digest []byte) error {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		INSERT INTO community_invitations (room_id, sender_id, recipient_id, token_digest, message, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, i.RoomID, i.SenderID, i.RecipientID, digest, i.Message, i.CreatedAt.Unix(), i.ExpiresAt.Unix())
	if err != nil {
		return err
	}
	i.ID, err = result.LastInsertId()
	return err
}

func (r *CommunityInvitationRepository) Get(ctx context.Context, id int64, lock bool) (*model.CommunityInvitation, error) {
	query := "SELECT " + invitationColumns + " FROM community_invitations i WHERE i.id = ?"
	if lock {
		query += " FOR UPDATE"
	}
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx, query, id))
}

func (r *CommunityInvitationRepository) GetByToken(ctx context.Context, digest []byte) (*model.CommunityInvitation, error) {
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx,
		"SELECT "+invitationColumns+" FROM community_invitations i WHERE i.token_digest = ?", digest))
}

func (r *CommunityInvitationRepository) Pending(ctx context.Context, roomID, recipientID int64, now time.Time) (*model.CommunityInvitation, error) {
	return scanInvitation(extractDB(ctx, r.DB).QueryRowContext(ctx,
		"SELECT "+invitationColumns+` FROM community_invitations i
		WHERE i.room_id = ? AND i.recipient_id = ? AND i.state = 'pending' AND i.expires_at > ?
		ORDER BY i.id DESC LIMIT 1 FOR UPDATE`, roomID, recipientID, now.Unix()))
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
	items, err := r.list(ctx, "SELECT "+invitationColumns+filter+" ORDER BY i.id DESC LIMIT ? OFFSET ?", recipientID, now.Unix(), limit, offset)
	return items, total, err
}

// ListSent はルームの招待を新しい順に返す。owner でなければ自分が作った招待だけ。
// history が false なら使える招待、true ならそれ以外（参加済み・辞退・取り消し・期限切れ）。
// 宛先がすでにメンバーになった個別の招待は「使える招待」から外し、履歴に回す。
func (r *CommunityInvitationRepository) ListSent(ctx context.Context, roomID, actorID int64, owner, history bool, now time.Time, limit, offset int32) ([]*model.CommunityInvitation, error) {
	active := activeInvitation + ` AND (i.recipient_id IS NULL OR NOT EXISTS (SELECT 1 FROM room_users WHERE room_id = i.room_id AND user_id = i.recipient_id))`
	if history {
		active = "NOT (" + active + ")"
	}
	return r.list(ctx, "SELECT "+invitationColumns+` FROM community_invitations i
		WHERE i.room_id = ? AND (i.sender_id = ? OR ?) AND `+active+`
		ORDER BY i.id DESC LIMIT ? OFFSET ?`, roomID, actorID, owner, now.Unix(), limit, offset)
}

// CandidateStates は招待の候補（検索結果）それぞれの状態を返す。載っていない人は招待できる。
func (r *CommunityInvitationRepository) CandidateStates(ctx context.Context, roomID, actorID int64, userIDs []int64, now time.Time) (map[int64]string, error) {
	states := make(map[int64]string, len(userIDs))
	if len(userIDs) == 0 {
		return states, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]any, 0, len(userIDs)+8)
	args = append(args, roomID, roomID, now.Unix(), roomID, now.Unix(), actorID, actorID, actorID)
	for _, id := range userIDs {
		args = append(args, id)
	}
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, `
		SELECT u.id,
			EXISTS (SELECT 1 FROM room_users ru WHERE ru.room_id = ? AND ru.user_id = u.id),
			EXISTS (SELECT 1 FROM community_invitations i WHERE i.room_id = ? AND i.recipient_id = u.id AND `+activeInvitation+`),
			EXISTS (SELECT 1 FROM community_invitations d WHERE d.room_id = ? AND d.recipient_id = u.id AND d.state = 'declined' AND d.expires_at > ?)
			OR EXISTS (SELECT 1 FROM blocks b WHERE (b.user_id = ? AND b.blocked_user_id = u.id) OR (b.user_id = u.id AND b.blocked_user_id = ?))
			OR u.id = ?
		FROM users u WHERE u.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var member, invited, unavailable bool
		if err := rows.Scan(&id, &member, &invited, &unavailable); err != nil {
			return nil, err
		}
		switch {
		case member:
			states[id] = model.CommunityCandidateMember
		case invited:
			states[id] = model.CommunityCandidateInvited
		case unavailable:
			states[id] = model.CommunityCandidateUnavailable
		}
	}
	return states, rows.Err()
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
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		"UPDATE community_invitations SET state = ?, responded_at = ? WHERE id = ?", state, time.Now().Unix(), id)
	return err
}

func (r *CommunityInvitationRepository) AcceptReceived(ctx context.Context, roomID, recipientID int64, now time.Time) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE community_invitations SET state = 'accepted', responded_at = ?
		WHERE room_id = ? AND recipient_id = ? AND state = 'pending' AND expires_at > ?`, now.Unix(), roomID, recipientID, now.Unix())
	return err
}

// RecordUse は招待 invitationID から userID が参加したことを残し、利用数を1増やす。
// 同じ人の2回目は数えない（招待の行は呼び出し側がロック済み）。
func (r *CommunityInvitationRepository) RecordUse(ctx context.Context, invitationID, userID int64, now time.Time) error {
	db := extractDB(ctx, r.DB)
	result, err := db.ExecContext(ctx,
		"INSERT IGNORE INTO community_invitation_uses (invitation_id, user_id, created_at) VALUES (?, ?, ?)",
		invitationID, userID, now.Unix())
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return err
	}
	_, err = db.ExecContext(ctx, "UPDATE community_invitations SET use_count = use_count + 1 WHERE id = ?", invitationID)
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

func (r *CommunityInvitationRepository) WelcomeTarget(ctx context.Context, roomID, viewerID int64) (*model.CommunityWelcomeTarget, error) {
	var target model.CommunityWelcomeTarget
	var sent bool
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `
		SELECT m.id, a.name, replies.sender_id IS NOT NULL
		FROM room_users viewer
		JOIN room_users joined ON joined.room_id = viewer.room_id AND joined.id > viewer.id
		JOIN user_accounts a ON a.user_id = joined.user_id
		JOIN users u ON u.id = a.user_id AND u.status = 'active'
		JOIN messages m ON m.id = joined.welcome_message_id AND m.deleted_at IS NULL
		LEFT JOIN community_welcome_replies replies ON replies.notice_message_id = m.id AND replies.sender_id = viewer.user_id
		WHERE viewer.room_id = ? AND viewer.user_id = ?
		  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE
		    (b.user_id = viewer.user_id AND b.blocked_user_id = joined.user_id) OR
		    (b.user_id = joined.user_id AND b.blocked_user_id = viewer.user_id))
		ORDER BY m.id DESC
		LIMIT 1`, roomID, viewerID).Scan(&target.MessageID, &target.UserName, &sent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil || sent {
		return nil, err
	}
	return &target, nil
}

func (r *CommunityInvitationRepository) SetWelcomeMessage(ctx context.Context, roomID, userID, messageID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		"UPDATE room_users SET welcome_message_id = ? WHERE room_id = ? AND user_id = ?", messageID, roomID, userID)
	return err
}

func (r *CommunityInvitationRepository) MarkWelcomeSent(ctx context.Context, noticeID, userID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		"INSERT INTO community_welcome_replies (notice_message_id, sender_id) VALUES (?, ?)", noticeID, userID)
	return err
}
