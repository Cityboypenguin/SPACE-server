package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	_ "github.com/go-sql-driver/mysql"
)

// DataLoader のバッチ取得は「単体取得を N 回呼ぶのと同じ結果を1〜2クエリで返す」
// ことが全てで、その保証はほぼ SQL の側にある（IN 句・行コンストラクタ・
// ROW_NUMBER のパーティション）。Go 側の fake では確かめようがないので、
// ここは実際の MySQL に投げる統合テストにしてある。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run Batch
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。
func batchTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed batch loader test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_batch_test_%d", os.Getpid())
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to create the throwaway schema: %v", err)
	}

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		root.Close()
		t.Fatalf("failed to open the throwaway schema: %v", err)
	}

	// db/migrations の DDL から外部キーだけ落とした形（users / rooms の行は作らない）。
	// 見たいのは取得結果の一致であって参照整合性ではない。
	ddl := []string{
		`CREATE TABLE rooms (
			id BIGINT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE room_anonymous_identities (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			label VARCHAR(50) NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_room_anon_room_user (room_id, user_id),
			UNIQUE KEY unique_room_anon_room_label (room_id, label)
		)`,
		`CREATE TABLE questions (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			asker_user_id BIGINT NOT NULL,
			author_role VARCHAR(20) NOT NULL DEFAULT 'STUDENT',
			body TEXT NOT NULL,
			is_answered BOOLEAN NOT NULL DEFAULT FALSE,
			best_answer_id BIGINT DEFAULT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			INDEX idx_questions_room (room_id)
		)`,
		`CREATE TABLE answers (
			id BIGINT NOT NULL AUTO_INCREMENT,
			question_id BIGINT NOT NULL,
			author_user_id BIGINT NOT NULL,
			author_role VARCHAR(20) NOT NULL DEFAULT 'STUDENT',
			body TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			INDEX idx_answers_question (question_id)
		)`,
		`CREATE TABLE answer_likes (
			id BIGINT NOT NULL AUTO_INCREMENT,
			answer_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_answer_likes_answer_user (answer_id, user_id)
		)`,
		`CREATE TABLE polls (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			author_user_id BIGINT NOT NULL,
			author_role VARCHAR(20) NOT NULL DEFAULT 'STUDENT',
			question VARCHAR(255) NOT NULL,
			allow_multiple_choice BOOLEAN NOT NULL DEFAULT FALSE,
			deadline BIGINT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE poll_options (
			id BIGINT NOT NULL AUTO_INCREMENT,
			poll_id BIGINT NOT NULL,
			label VARCHAR(255) NOT NULL,
			display_order INT NOT NULL DEFAULT 0,
			PRIMARY KEY (id),
			INDEX idx_poll_options_poll (poll_id)
		)`,
		`CREATE TABLE poll_votes (
			id BIGINT NOT NULL AUTO_INCREMENT,
			poll_option_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_poll_votes_option_user (poll_option_id, user_id)
		)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Close()
			t.Fatalf("failed to create a test table: %v", err)
		}
	}

	return db, func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("failed to clean up the throwaway schema %s: %v", schema, err)
		}
		root.Close()
	}
}

func TestBatchGetRoomsByIDs_MatchesTheSingleFetch(t *testing.T) {
	db, cleanup := batchTestDB(t)
	defer cleanup()

	now := time.Now().Unix()
	for i := int64(1); i <= 3; i++ {
		mustExec(t, db, `INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			i, fmt.Sprintf("room %d", i), model.RoomTypeCourse, now, now)
	}

	repo := NewMySQLRoomRepository(db)
	ctx := context.Background()

	// 存在しない 99 を混ぜる。バッチが「見つからないIDは map から落とす」
	// 約束を守っていないと、ここで nil が入ったり件数が合わなくなる。
	got, err := repo.GetRoomsByIDs(ctx, []int64{1, 2, 3, 99})
	if err != nil {
		t.Fatalf("GetRoomsByIDs: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (存在しないIDは落とすこと)", len(got))
	}
	if _, ok := got[99]; ok {
		t.Fatal("存在しないIDが map に入っている")
	}
	for i := int64(1); i <= 3; i++ {
		want, err := repo.GetRoomByID(ctx, i)
		if err != nil {
			t.Fatalf("GetRoomByID: %v", err)
		}
		if got[i] == nil || *got[i] != *want {
			t.Fatalf("room %d: batch = %+v, single = %+v", i, got[i], want)
		}
	}
}

func TestBatchGetAnonymousIdentitiesByRoomUserKeys_MatchesTheSingleFetch(t *testing.T) {
	db, cleanup := batchTestDB(t)
	defer cleanup()

	now := time.Now().Unix()
	rows := []struct {
		id, roomID, userID int64
		label              string
	}{
		{1, 10, 100, "匿名001"},
		{2, 10, 200, "匿名002"},
		{3, 20, 100, "匿名001"},
	}
	for _, r := range rows {
		mustExec(t, db, `INSERT INTO room_anonymous_identities (id, room_id, user_id, label, created_at) VALUES (?, ?, ?, ?, ?)`,
			r.id, r.roomID, r.userID, r.label, now)
	}

	repo := NewMySQLRoomAnonymousIdentityRepository(db)
	ctx := context.Background()

	// (10,100) と (20,100) は user_id が同じ、(10,100) と (10,200) は room_id が同じ。
	// 行コンストラクタの IN が組でマッチしていないと、ここで取り違えが起きる。
	// (20,200) は行が無いケース。
	keys := []repository.RoomUserKey{
		{RoomID: 10, UserID: 100},
		{RoomID: 10, UserID: 200},
		{RoomID: 20, UserID: 100},
		{RoomID: 20, UserID: 200},
	}
	got, err := repo.GetByRoomUserKeys(ctx, keys)
	if err != nil {
		t.Fatalf("GetByRoomUserKeys: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (行の無い key は落とすこと)", len(got))
	}
	if _, ok := got[repository.RoomUserKey{RoomID: 20, UserID: 200}]; ok {
		t.Fatal("行の無い key が map に入っている")
	}
	for _, k := range keys {
		want, err := repo.Get(ctx, k.RoomID, k.UserID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if want == nil {
			continue
		}
		if got[k] == nil || got[k].ID != want.ID || got[k].Label != want.Label {
			t.Fatalf("key %+v: batch = %+v, single = %+v", k, got[k], want)
		}
	}
}

func TestBatchPollResults_MatchTheSingleFetch(t *testing.T) {
	db, cleanup := batchTestDB(t)
	defer cleanup()

	now := time.Now().Unix()
	for _, pollID := range []int64{1, 2, 3} {
		mustExec(t, db, `INSERT INTO polls (id, room_id, author_user_id, question, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			pollID, 10, 100, fmt.Sprintf("q%d", pollID), now, now)
	}
	// display_order をわざと挿入順と逆に入れて、並びが display_order で決まることを見る。
	options := []struct {
		id, pollID int64
		label      string
		order      int
	}{
		{11, 1, "B", 2}, {12, 1, "A", 1},
		{21, 2, "only", 1},
		// poll 3 は選択肢なし（バッチの map に現れない側）。
	}
	for _, o := range options {
		mustExec(t, db, `INSERT INTO poll_options (id, poll_id, label, display_order) VALUES (?, ?, ?, ?)`,
			o.id, o.pollID, o.label, o.order)
	}
	// 複数選択でも投票者は1人と数えること（CountVoters の約束）を跨いで確かめる。
	votes := []struct{ optionID, userID int64 }{
		{11, 100}, {12, 100}, // 同じ人が poll 1 の2択に投票
		{11, 200},
		{21, 300},
	}
	for _, v := range votes {
		mustExec(t, db, `INSERT INTO poll_votes (poll_option_id, user_id, created_at) VALUES (?, ?, ?)`, v.optionID, v.userID, now)
	}

	repo := NewMySQLPollRepository(db)
	ctx := context.Background()
	const viewer int64 = 100

	pollIDs := []int64{1, 2, 3}

	gotOptions, err := repo.ListOptionsWithResultsByPollIDs(ctx, pollIDs, viewer)
	if err != nil {
		t.Fatalf("ListOptionsWithResultsByPollIDs: %v", err)
	}
	for _, pollID := range pollIDs {
		want, err := repo.ListOptionsWithResults(ctx, pollID, viewer)
		if err != nil {
			t.Fatalf("ListOptionsWithResults: %v", err)
		}
		got := gotOptions[pollID]
		if len(got) != len(want) {
			t.Fatalf("poll %d: batch returned %d options, single returned %d", pollID, len(got), len(want))
		}
		for i := range want {
			// 並び（display_order）も含めて一致していること。
			if got[i].Option.ID != want[i].Option.ID ||
				got[i].VoteCount != want[i].VoteCount ||
				got[i].VotedByMe != want[i].VotedByMe {
				t.Fatalf("poll %d option %d: batch = %+v/%+v, single = %+v/%+v",
					pollID, i, got[i].Option, got[i].VoteCount, want[i].Option, want[i].VoteCount)
			}
		}
	}

	gotCounts, err := repo.CountVotersByPollIDs(ctx, pollIDs)
	if err != nil {
		t.Fatalf("CountVotersByPollIDs: %v", err)
	}
	for _, pollID := range pollIDs {
		want, err := repo.CountVoters(ctx, pollID)
		if err != nil {
			t.Fatalf("CountVoters: %v", err)
		}
		// 投票ゼロの poll は map に現れない。ゼロ値がそのまま正解になる。
		if gotCounts[pollID] != want {
			t.Fatalf("poll %d: batch voter count = %d, single = %d", pollID, gotCounts[pollID], want)
		}
	}
	if gotCounts[1] != 2 {
		t.Fatalf("poll 1 voter count = %d, want 2 (複数選択でも1人は1人)", gotCounts[1])
	}
}

func TestBatchAnswerFetches_MatchTheSingleFetch(t *testing.T) {
	db, cleanup := batchTestDB(t)
	defer cleanup()

	// 本文は暗号化して保存されるので、リポジトリが鍵を読めるようにする。
	t.Setenv("MESSAGE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	answerRepo, err := NewMySQLAnswerRepository(db)
	if err != nil {
		t.Fatalf("NewMySQLAnswerRepository: %v", err)
	}
	questionRepo, err := NewMySQLQuestionRepository(db)
	if err != nil {
		t.Fatalf("NewMySQLQuestionRepository: %v", err)
	}
	ctx := context.Background()
	const viewer int64 = 100

	now := time.Now().Unix()

	// 本文は暗号化列なので、直接 INSERT せずリポジトリ経由で作る（復号できなくなる）。
	var questionIDs []int64
	for i := 1; i <= 3; i++ {
		q := &model.Question{RoomID: 10, AskerUserID: 100, AuthorRole: "STUDENT", Body: fmt.Sprintf("question %d", i)}
		if err := questionRepo.SaveQuestion(ctx, q); err != nil {
			t.Fatalf("SaveQuestion: %v", err)
		}
		questionIDs = append(questionIDs, q.ID)
	}

	// 質問1にいいね数の違う回答を5件。like_count 降順・created_at 昇順・id 昇順という
	// 並びが、窓関数版でも一字一句同じであることを見たい。
	type seed struct {
		questionID int64
		likes      []int64 // いいねした user_id
		createdAt  int64
	}
	seeds := []seed{
		{questionIDs[0], []int64{1, 2, 3}, now + 5}, // 3いいね
		{questionIDs[0], nil, now},                  // いいね0・最古
		{questionIDs[0], []int64{1, 2}, now + 5},    // 2いいね
		{questionIDs[0], []int64{100}, now + 1},     // viewer がいいね（LikedByMe）
		{questionIDs[0], nil, now + 2},              // いいね0・新しい（同数のタイブレーク）
		{questionIDs[1], []int64{5}, now},
		// questionIDs[2] は回答なし。
	}
	var answerIDs []int64
	for _, s := range seeds {
		a := &model.Answer{QuestionID: s.questionID, AuthorUserID: 7, AuthorRole: "STUDENT", Body: "answer body"}
		if err := answerRepo.SaveAnswer(ctx, a); err != nil {
			t.Fatalf("SaveAnswer: %v", err)
		}
		// SaveAnswer は created_at に現在時刻を入れるので、いいね数が同じときの
		// タイブレーク（created_at 昇順）を試せるよう後からずらす。
		mustExec(t, db, `UPDATE answers SET created_at = ? WHERE id = ?`, s.createdAt, a.ID)
		answerIDs = append(answerIDs, a.ID)
		for _, uid := range s.likes {
			mustExec(t, db, `INSERT INTO answer_likes (answer_id, user_id, created_at) VALUES (?, ?, ?)`, a.ID, uid, now)
		}
	}

	t.Run("GetAnswersWithLikesByIDs", func(t *testing.T) {
		got, err := answerRepo.GetAnswersWithLikesByIDs(ctx, append(answerIDs, 9999), viewer)
		if err != nil {
			t.Fatalf("GetAnswersWithLikesByIDs: %v", err)
		}
		if _, ok := got[9999]; ok {
			t.Fatal("存在しないIDが map に入っている")
		}
		for _, id := range answerIDs {
			want, err := answerRepo.GetAnswerWithLikesByID(ctx, id, viewer)
			if err != nil {
				t.Fatalf("GetAnswerWithLikesByID: %v", err)
			}
			g := got[id]
			if g == nil || g.LikeCount != want.LikeCount || g.LikedByMe != want.LikedByMe || g.Answer.Body != want.Answer.Body {
				t.Fatalf("answer %d: batch = %+v, single = %+v", id, g, want)
			}
		}
	})

	t.Run("ListAnswerPagesByQuestionIDs", func(t *testing.T) {
		// 1ページ目と、途中から切ったページの両方で単体版と突き合わせる。
		for _, page := range []struct{ limit, offset int }{{20, 0}, {2, 0}, {2, 2}, {2, 10}} {
			got, err := answerRepo.ListAnswerPagesByQuestionIDs(ctx, questionIDs, viewer, repository.PageQuery{Limit: page.limit, Offset: page.offset, WithTotal: true})
			if err != nil {
				t.Fatalf("ListAnswerPagesByQuestionIDs: %v", err)
			}
			for _, qid := range questionIDs {
				wantItems, err := answerRepo.ListAnswersWithLikesByQuestionID(ctx, qid, viewer, page.limit, page.offset)
				if err != nil {
					t.Fatalf("ListAnswersWithLikesByQuestionID: %v", err)
				}
				wantTotal, err := answerRepo.CountAnswersByQuestionID(ctx, qid)
				if err != nil {
					t.Fatalf("CountAnswersByQuestionID: %v", err)
				}

				g := got[qid]
				if g == nil {
					t.Fatalf("question %d: batch returned no page (回答ゼロでも空ページを返すこと)", qid)
				}
				if g.Total != wantTotal {
					t.Fatalf("question %d limit=%d offset=%d: batch total = %d, single = %d",
						qid, page.limit, page.offset, g.Total, wantTotal)
				}
				if len(g.Items) != len(wantItems) {
					t.Fatalf("question %d limit=%d offset=%d: batch returned %d items, single returned %d",
						qid, page.limit, page.offset, len(g.Items), len(wantItems))
				}
				for i := range wantItems {
					// 並びまで一致していること。ここがずれると、同じ引数なのに
					// 返る回答が変わる（ROW_NUMBER の ORDER BY が単体版と違う）。
					if g.Items[i].Answer.ID != wantItems[i].Answer.ID ||
						g.Items[i].LikeCount != wantItems[i].LikeCount ||
						g.Items[i].LikedByMe != wantItems[i].LikedByMe {
						t.Fatalf("question %d limit=%d offset=%d item %d: batch = %+v, single = %+v",
							qid, page.limit, page.offset, i, g.Items[i], wantItems[i])
					}
				}
			}
		}
	})

	t.Run("GetQuestionsByIDs", func(t *testing.T) {
		got, err := questionRepo.GetQuestionsByIDs(ctx, append(append([]int64{}, questionIDs...), 9999))
		if err != nil {
			t.Fatalf("GetQuestionsByIDs: %v", err)
		}
		if _, ok := got[9999]; ok {
			t.Fatal("存在しないIDが map に入っている")
		}
		for _, qid := range questionIDs {
			want, err := questionRepo.GetQuestionByID(ctx, qid)
			if err != nil {
				t.Fatalf("GetQuestionByID: %v", err)
			}
			g := got[qid]
			// 本文は暗号化列なので、復号まで単体版と同じ経路を通っていることを見る。
			if g == nil || g.RoomID != want.RoomID || g.Body != want.Body {
				t.Fatalf("question %d: batch = %+v, single = %+v", qid, g, want)
			}
		}
	})
}
