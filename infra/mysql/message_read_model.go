package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// repository.MessageReadModel の実装。
// ここは「与えられた条件を SQL に落とす」だけで、どのカーソルをどう使うかという
// 仕様の説明は usecase/message/list_messages.go 側に置いている。

// defaultMessageLimit は Limit 未指定（0以下）で呼ばれたときの保険。
// 上限のクランプは GraphQL 層で行う。
const defaultMessageLimit = 50

// resolveCursor はカーソルが複数指定されたときの優先順位（AfterTime > AfterID >
// BeforeID）を1箇所に固定する。クエリ組み立てと hasMore 判定が別々の解釈をすると
// 境界で嘘をつくため、両方がこの結果だけを見るようにしている。
func resolveCursor(c repository.MessageCursor) repository.MessageCursor {
	switch {
	case c.AfterTime != nil:
		return repository.MessageCursor{AfterTime: c.AfterTime}
	case c.AfterID != nil:
		return repository.MessageCursor{AfterID: c.AfterID}
	case c.BeforeID != nil:
		return repository.MessageCursor{BeforeID: c.BeforeID}
	default:
		return repository.MessageCursor{}
	}
}

// boundaryProbe は「この条件に合う未削除メッセージが1件でもあるか」を DB に聞くための条件。
// column / op は下のコードが決め打ちする値だけで、外部入力は value（プレースホルダ）に入る。
type boundaryProbe struct {
	column string // "id" または "created_at"
	op     string // "<" / "<=" / ">" / ">="
	value  int64
}

// olderBoundary はページより古い側に何かあるかを調べるための条件を返す。
// nil は「調べるまでもなく無い」。
//
// 1件でも取れていればページ先頭より古いものを探せばよい。0件のときは
// ページの位置がカーソルでしか決まらないので、カーソルの位置を基準に判定する
// （以前は after 系で hasMoreBefore を true 固定しており、部屋の先頭でも
// 「まだ古いものがある」と嘘をついていた）。
func olderBoundary(items []*model.Message, cursor repository.MessageCursor) *boundaryProbe {
	if len(items) > 0 {
		return &boundaryProbe{column: "id", op: "<", value: items[0].ID}
	}
	c := resolveCursor(cursor)
	switch {
	case c.AfterTime != nil:
		return &boundaryProbe{column: "created_at", op: "<=", value: c.AfterTime.Unix()}
	case c.AfterID != nil:
		return &boundaryProbe{column: "id", op: "<=", value: *c.AfterID}
	case c.BeforeID != nil:
		// beforeID より古いものを探して0件だった＝それより古いものは無い。
		return nil
	default:
		// カーソル無しで0件＝部屋にメッセージが無い。
		return nil
	}
}

// newerBoundary はページより新しい側に何かあるかを調べるための条件を返す。
// nil は「調べるまでもなく無い」。
func newerBoundary(items []*model.Message, cursor repository.MessageCursor) *boundaryProbe {
	if len(items) > 0 {
		return &boundaryProbe{column: "id", op: ">", value: items[len(items)-1].ID}
	}
	c := resolveCursor(cursor)
	switch {
	case c.BeforeID != nil:
		// beforeID 自身とそれより新しいものは（消えていなければ）新しい側にある。
		return &boundaryProbe{column: "id", op: ">=", value: *c.BeforeID}
	default:
		// after 系で0件＝それより新しいものは無い。カーソル無しで0件なら部屋が空。
		return nil
	}
}

// buildMessagePageQuery はカーソルから SELECT を組み立てる。
// ascOrder が false のときは DESC で引いているので、呼び出し側で昇順に直す。
func buildMessagePageQuery(roomID int64, limit int, cursor repository.MessageCursor) (query string, args []interface{}, ascOrder bool) {
	c := resolveCursor(cursor)
	switch {
	case c.AfterTime != nil:
		// 未読起点: afterTime より新しいメッセージを昇順で取得
		query = fmt.Sprintf(`SELECT %s FROM messages
			WHERE room_id = ? AND created_at > ? AND deleted_at IS NULL ORDER BY id ASC LIMIT ?`, messageColumns)
		return query, []interface{}{roomID, c.AfterTime.Unix(), limit + 1}, true
	case c.AfterID != nil:
		// 新着ページング: afterID より新しいメッセージを昇順で取得
		query = fmt.Sprintf(`SELECT %s FROM messages
			WHERE room_id = ? AND id > ? AND deleted_at IS NULL ORDER BY id ASC LIMIT ?`, messageColumns)
		return query, []interface{}{roomID, *c.AfterID, limit + 1}, true
	case c.BeforeID != nil:
		// 過去ページング: beforeID より古いメッセージを降順で取得して反転
		query = fmt.Sprintf(`SELECT %s FROM messages
			WHERE room_id = ? AND id < ? AND deleted_at IS NULL ORDER BY id DESC LIMIT ?`, messageColumns)
		return query, []interface{}{roomID, *c.BeforeID, limit + 1}, false
	default:
		// 初回ロード（未読なし）: 最新メッセージを降順で取得して反転
		query = fmt.Sprintf(`SELECT %s FROM messages
			WHERE room_id = ? AND deleted_at IS NULL ORDER BY id DESC LIMIT ?`, messageColumns)
		return query, []interface{}{roomID, limit + 1}, false
	}
}

func (r *MySQLMessageRepository) ListMessagesByRoomID(ctx context.Context, q repository.MessageQuery) (*repository.MessagePage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultMessageLimit
	}

	query, args, ascOrder := buildMessagePageQuery(q.RoomID, limit, q.Cursor)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*model.Message
	for rows.Next() {
		m, err := r.scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// limit+1 件引いているので、余りは捨てる（DESC のときは反転前に捨てる＝
	// カーソルに近い側を残す）。
	if len(messages) > limit {
		messages = messages[:limit]
	}
	if !ascOrder {
		// DESC で取得したので昇順（古い順）に戻す
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	hasMoreBefore, err := r.messageExists(ctx, q.RoomID, olderBoundary(messages, q.Cursor))
	if err != nil {
		return nil, err
	}
	hasMoreAfter, err := r.messageExists(ctx, q.RoomID, newerBoundary(messages, q.Cursor))
	if err != nil {
		return nil, err
	}

	return &repository.MessagePage{Items: messages, HasMoreBefore: hasMoreBefore, HasMoreAfter: hasMoreAfter}, nil
}

// messageExists は probe の条件に合う未削除メッセージが1件でもあるかを返す。
// limit+1 件取れたかどうかでの推定ではなく、毎回この軽いクエリで実測する。
func (r *MySQLMessageRepository) messageExists(ctx context.Context, roomID int64, probe *boundaryProbe) (bool, error) {
	if probe == nil {
		return false, nil
	}
	// column / op は boundaryProbe を作る箇所の定数のみ。値は必ずプレースホルダで渡す。
	query := fmt.Sprintf(
		`SELECT 1 FROM messages WHERE room_id = ? AND %s %s ? AND deleted_at IS NULL LIMIT 1`,
		probe.column, probe.op,
	)
	var exists int
	err := r.DB.QueryRowContext(ctx, query, roomID, probe.value).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *MySQLMessageRepository) GetLastMessagesByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error) {
	result := make(map[int64]*model.Message)
	if len(roomIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(`
		SELECT %s
		FROM messages m
		INNER JOIN (
			SELECT room_id, MAX(id) AS max_id
			FROM messages
			WHERE room_id IN (%s) AND deleted_at IS NULL
			GROUP BY room_id
		) latest ON m.room_id = latest.room_id AND m.id = latest.max_id
	`, qualifiedMessageColumns("m"), placeholders)

	args := make([]interface{}, len(roomIDs))
	for i, id := range roomIDs {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		m, err := r.scanMessage(rows)
		if err != nil {
			return nil, err
		}
		result[m.RoomID] = m
	}
	return result, rows.Err()
}

// GetLatestMessageID はルームの最新メッセージIDを返す（1件も無ければ nil）。
//
// deleted_at IS NULL で絞らないのは意図的。返り値は既読位置（しおり）として
// 保存するもので、末尾のメッセージがソフトデリート済みだという理由で1つ手前の ID を
// 返すと、その削除済みメッセージより後に来た行が既読位置の後ろに残り、既読にした
// はずのものが未読へ戻る。
func (r *MySQLMessageRepository) GetLatestMessageID(ctx context.Context, roomID int64) (*int64, error) {
	var latestID sql.NullInt64
	if err := r.DB.QueryRowContext(ctx,
		`SELECT MAX(id) FROM messages WHERE room_id = ?`, roomID,
	).Scan(&latestID); err != nil {
		return nil, err
	}
	if !latestID.Valid {
		return nil, nil
	}
	id := latestID.Int64
	return &id, nil
}
