package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.RoomAnonymousIdentityRepository = &MySQLRoomAnonymousIdentityRepository{}

type MySQLRoomAnonymousIdentityRepository struct {
	DB *sql.DB
}

func NewMySQLRoomAnonymousIdentityRepository(db *sql.DB) repository.RoomAnonymousIdentityRepository {
	return &MySQLRoomAnonymousIdentityRepository{DB: db}
}

// GetOrCreate は (roomID, userID) の匿名IDを返し、無ければ採番して作る。
//
// 採番は room_anonymous_sequences のカウンタ1文で原子的に進める（allocateSequence）。
// 以前は「この表を COUNT(*) して +1」し、それを MySQL の名前付きロック
// (GET_LOCK/RELEASE_LOCK) で守っていたが、次の2つが壊れていた。
//
//   - 名前付きロックは接続単位なのに、sql.DB は文ごとにプールの別接続へ振り分けうる。
//     「接続Aで GET_LOCK → 接続Bで採番 → 接続Cで RELEASE_LOCK」となると排他にならず、
//     さらに接続Aのロックが解放されないまま残る。
//   - COUNT(*) は行が減ると小さくなる。ユーザー削除で identity 行が CASCADE 削除
//     されると件数が戻り、既存の「匿名005」と同じ番号を再発行してしまう。
//
// カウンタ方式なら排他も単調増加も DB の1文が保証するので、ロックは要らなくなった。
//
// 欠番について。採番してから INSERT するまでの間に同じ利用者の別リクエストが
// 先に行を作ると、こちらの INSERT は unique (room_id, user_id) に負けて採番した
// 番号が捨てられ、番号に欠番ができる。これは許容する。番号は「同じ部屋の中で
// 誰か1人を指す記号」でしかなく、連番であることに意味は無い。欠番が出ても
// (a) 同じ番号が2人に渡ることは無い（カウンタは単調増加）し、
// (b) 番号から人数や個人が分かるわけでもない（むしろ連番でない方が漏れにくい）ので、
// 匿名性も一意性も壊れない。
//
// 既読位置は別表（course_room_reads）なので、この表に行があるのは
// 「そのルームで投稿したことがある」ときだけ。行の有無だけ見れば済む。
func (r *MySQLRoomAnonymousIdentityRepository) GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	if existing, err := r.Get(ctx, roomID, userID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	seq, err := r.allocateSequence(ctx, roomID)
	if err != nil {
		return nil, err
	}

	label := fmt.Sprintf("匿名%03d", seq)
	now := time.Now()

	result, err := r.DB.ExecContext(ctx,
		`INSERT INTO room_anonymous_identities (room_id, user_id, label, created_at) VALUES (?, ?, ?, ?)`,
		roomID, userID, label, now.Unix(),
	)
	if err != nil {
		// 同時投稿で先に行ができていた場合（unique (room_id,user_id) 違反）は、
		// 先勝ちした番号こそがその人の匿名IDなので、それを読み直して返す。
		// 行が無いなら別の失敗（例: unique (room_id,label) の安全網に当たった）なので、
		// 重複したラベルを使い回さずそのまま失敗させる。
		if existing, getErr := r.Get(ctx, roomID, userID); getErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &model.RoomAnonymousIdentity{ID: id, RoomID: roomID, UserID: userID, Label: label, CreatedAt: now}, nil
}

// allocateSequence は roomID の採番カウンタを1つ進め、確定した番号を返す。
//
// 1文で「行が無ければ1番を配って next_seq=2 から始める / あれば現在値を配って +1 する」
// を済ませる。値は SELECT LAST_INSERT_ID() では読まない（別接続に振り分けられると
// 別の文の結果を読んでしまう）。この文自体の OK パケットに載る値を
// Result.LastInsertId() で受け取るので、プールの振り分けに影響されない。
//
// 新規 INSERT された経路では 0 が返る。room_anonymous_sequences に AUTO_INCREMENT 列が
// 無く、その経路では LAST_INSERT_ID(expr) を通らないため。配った番号は 1 固定なので、
// 0 はそのまま「1番を配った」と読める（ON DUPLICATE 経路が返す値は必ず 1 以上）。
func (r *MySQLRoomAnonymousIdentityRepository) allocateSequence(ctx context.Context, roomID int64) (int64, error) {
	now := time.Now().Unix()
	result, err := r.DB.ExecContext(ctx,
		`INSERT INTO room_anonymous_sequences (room_id, next_seq, created_at, updated_at)
		 VALUES (?, 2, ?, ?)
		 ON DUPLICATE KEY UPDATE next_seq = LAST_INSERT_ID(next_seq) + 1, updated_at = ?`,
		roomID, now, now, now,
	)
	if err != nil {
		return 0, err
	}
	allocated, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if allocated == 0 {
		return 1, nil
	}
	return allocated, nil
}

// Get returns the row for (roomID, userID), or nil when the user has never posted
// in the room. 番号を割り当てずに引きたい表示側のための口。
func (r *MySQLRoomAnonymousIdentityRepository) Get(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, room_id, user_id, label, created_at FROM room_anonymous_identities WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	)
	var identity model.RoomAnonymousIdentity
	var createdAt int64
	// label は NOT NULL（行ができるのは投稿時で、そのとき必ず採番する）。
	if err := row.Scan(&identity.ID, &identity.RoomID, &identity.UserID, &identity.Label, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	identity.CreatedAt = time.Unix(createdAt, 0)
	return &identity, nil
}

// GetByRoomUserKeys は Get の一括版。DataLoader から1クエリでまとめて呼ばれる。
//
// (room_id, user_id) の組で引くので、行コンストラクタ
// `WHERE (room_id, user_id) IN ((?,?), ...)` を使う。unique (room_id, user_id) が
// そのまま効くので、件数が増えても1回のインデックス走査で済む。
//
// 行が無い key は map に入れない（Get が nil, nil を返すのと同じ扱い）。
// 呼び出し側はそれを「実名にフォールバックしてよい」と読んではいけない。
// 授業ルームなら番号なしの「匿名」へ倒すこと（anonymousPlaceholderUser を参照）。
func (r *MySQLRoomAnonymousIdentityRepository) GetByRoomUserKeys(ctx context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
	result := make(map[repository.RoomUserKey]*model.RoomAnonymousIdentity, len(keys))
	if len(keys) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("(?,?),", len(keys)), ",")
	query := fmt.Sprintf(`
		SELECT id, room_id, user_id, label, created_at
		FROM room_anonymous_identities
		WHERE (room_id, user_id) IN (%s)
	`, placeholders)

	args := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		args = append(args, k.RoomID, k.UserID)
	}

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var identity model.RoomAnonymousIdentity
		var createdAt int64
		if err := rows.Scan(&identity.ID, &identity.RoomID, &identity.UserID, &identity.Label, &createdAt); err != nil {
			return nil, err
		}
		identity.CreatedAt = time.Unix(createdAt, 0)
		result[repository.RoomUserKey{RoomID: identity.RoomID, UserID: identity.UserID}] = &identity
	}
	return result, rows.Err()
}
