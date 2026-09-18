package mysql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// UniqueDMSenders は「DM を送ったことのある実ユーザー数」だが、集計は Go では
// なく1本の SQL（rooms との JOIN と deleted_at の除外）なので、実際に MySQL へ
// 投げないと「コミュニティの発言者まで数えていないか」「削除済みしか無いユーザーを
// 数えていないか」を確かめられない。そのため DB を使う統合テストにしてある。
// 使い捨てスキーマの作り方は throwawaySchemaDB（testdb_test.go）に揃えてある。
func dmSendersTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	return throwawaySchemaDB(t, "space_analytics_test", []string{
		`CREATE TABLE rooms (
			id BIGINT NOT NULL AUTO_INCREMENT,
			type VARCHAR(50) NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE messages (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			deleted_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
	})
}

func TestUniqueDMSenders_CountsOnlyLiveDMSenders(t *testing.T) {
	db, cleanup := dmSendersTestDB(t)
	defer cleanup()

	const (
		dmRoom        = int64(1)
		otherDMRoom   = int64(2)
		communityRoom = int64(3)
		courseRoom    = int64(4)
	)
	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'dm'), (?, 'dm'), (?, 'community'), (?, 'course')`,
		dmRoom, otherDMRoom, communityRoom, courseRoom)

	const (
		alice   = int64(10) // DM を2通（同じ人は1回だけ数える）
		bob     = int64(11) // 別の DM ルームで1通
		carol   = int64(12) // コミュニティだけ
		dave    = int64(13) // 授業チャットだけ
		erin    = int64(14) // DM に投稿したが削除済みしか無い
		frank   = int64(15) // DM に2通、うち1通削除済み（生存が1通あるので数える）
		now     = int64(1700000000)
		deleted = int64(1700000001)
	)
	mustExec(t, db, `
		INSERT INTO messages (room_id, user_id, created_at, deleted_at) VALUES
			(?, ?, ?, NULL),
			(?, ?, ?, NULL),
			(?, ?, ?, NULL),
			(?, ?, ?, NULL),
			(?, ?, ?, NULL),
			(?, ?, ?, ?),
			(?, ?, ?, NULL),
			(?, ?, ?, ?)`,
		dmRoom, alice, now,
		dmRoom, alice, now,
		otherDMRoom, bob, now,
		communityRoom, carol, now,
		courseRoom, dave, now,
		dmRoom, erin, now, deleted,
		dmRoom, frank, now,
		dmRoom, frank, now, deleted,
	)

	var got int
	if err := db.QueryRowContext(context.Background(), uniqueDMSendersQuery, model.RoomTypeDM).Scan(&got); err != nil {
		t.Fatalf("failed to run the DM sender count: %v", err)
	}

	// alice と bob と frank の3人だけ。carol/dave は DM ではないルーム、
	// erin は削除済みメッセージしか持たない。
	if got != 3 {
		t.Fatalf("UniqueDMSenders = %d, want 3 (alice, bob, frank)", got)
	}

	// 修正前の SQL がこのデータで何を返していたかも残しておく: 6 人（全ルーム・
	// 削除込み）で、名前と実態が食い違っていたことがこの差に出る。
	var legacy int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(DISTINCT user_id) FROM messages`).Scan(&legacy); err != nil {
		t.Fatalf("failed to run the legacy count: %v", err)
	}
	if legacy != 6 {
		t.Fatalf("legacy count = %d, want 6; the fixture no longer demonstrates the bug", legacy)
	}
}

// ■ 集計まるごとを実 MySQL に当てるための土台
//
// GetAnalyticsSummary / GetTimeSeries / GetCommunityAnalytics は「どの行を数え
// ないか」が仕様そのもの（論理削除の除外）なので、Go 側のモックでは確かめられ
// ない。ここでは集計が読む列だけを持つ使い捨てスキーマを作って、集計を実際に
// 走らせる。外部キーと索引は落としてある（確かめたいのは SQL の意味であって
// スキーマではない）。
func analyticsSchemaDDL() []string {
	return []string{
		`CREATE TABLE users (
			id BIGINT NOT NULL AUTO_INCREMENT,
			created_at BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			last_active_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE posts (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			parent_id BIGINT NULL,
			created_at BIGINT NOT NULL,
			deleted_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE post_media (post_id BIGINT NOT NULL, media_id BIGINT NOT NULL)`,
		`CREATE TABLE media (id BIGINT NOT NULL, content_type VARCHAR(100) NOT NULL, PRIMARY KEY (id))`,
		`CREATE TABLE favorites (id BIGINT NOT NULL AUTO_INCREMENT, created_at BIGINT NOT NULL, PRIMARY KEY (id))`,
		`CREATE TABLE favorite_users (id BIGINT NOT NULL AUTO_INCREMENT, created_at BIGINT NOT NULL, PRIMARY KEY (id))`,
		`CREATE TABLE rooms (id BIGINT NOT NULL AUTO_INCREMENT, type VARCHAR(50) NOT NULL, PRIMARY KEY (id))`,
		`CREATE TABLE room_users (room_id BIGINT NOT NULL, user_id BIGINT NOT NULL)`,
		`CREATE TABLE communities (
			id BIGINT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			room_id BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE messages (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			deleted_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE user_reports (id BIGINT NOT NULL AUTO_INCREMENT, status VARCHAR(32) NOT NULL, PRIMARY KEY (id))`,
		`CREATE TABLE blocks (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`,
		`CREATE TABLE inquiries (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`,
		`CREATE TABLE profiles (user_id BIGINT NOT NULL, avatar_media_id BIGINT NULL)`,
		`CREATE TABLE notifications (id BIGINT NOT NULL AUTO_INCREMENT, is_read BOOLEAN NOT NULL DEFAULT FALSE, PRIMARY KEY (id))`,
		`CREATE TABLE user_session_summaries (
			user_id BIGINT NOT NULL,
			session_count INT NOT NULL DEFAULT 0,
			total_duration_seconds BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE page_view_stats (
			page_path VARCHAR(255) NOT NULL,
			view_count INT NOT NULL DEFAULT 0,
			total_duration_seconds BIGINT NOT NULL DEFAULT 0,
			total_max_scroll_depth DOUBLE NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE user_activity_dates (user_id BIGINT NOT NULL, activity_date DATE NOT NULL)`,
	}
}

func analyticsTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	return throwawaySchemaDB(t, "space_analytics_full_test", analyticsSchemaDDL())
}

// 論理削除済みメッセージが、メッセージ系の集計から一律に外れること。
//
// posts 系は前から deleted_at IS NULL で外していたのに messages 系は外して
// おらず、「削除したのに総メッセージ数が減らない」状態だった。揃えた結果として
// 下がる数字を、ここでまとめて固定する。
func TestAnalyticsExcludesDeletedMessages(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location()).Unix()
	const (
		liveRoom    = int64(1) // 生きたメッセージがあるコミュニティ
		deletedRoom = int64(2) // 削除済みメッセージしか無いコミュニティ
	)
	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'community'), (?, 'community')`, liveRoom, deletedRoom)
	mustExec(t, db, `INSERT INTO communities (id, name, room_id) VALUES (1, 'live', ?), (2, 'dead', ?)`, liveRoom, deletedRoom)

	// liveRoom: 生存2件 + 削除1件 / deletedRoom: 削除2件のみ
	mustExec(t, db, `
		INSERT INTO messages (room_id, user_id, created_at, deleted_at) VALUES
			(?, 100, ?, NULL),
			(?, 100, ?, NULL),
			(?, 101, ?, ?),
			(?, 102, ?, ?),
			(?, 103, ?, ?)`,
		liveRoom, today,
		liveRoom, today,
		liveRoom, today, today,
		deletedRoom, today, today,
		deletedRoom, today, today)

	repo := NewMySQLAnalyticsRepository(db)
	s, err := repo.GetAnalyticsSummary(context.Background(), repository.AllFields())
	if err != nil {
		t.Fatalf("GetAnalyticsSummary failed: %v", err)
	}

	if s.TotalMessages != 2 {
		t.Errorf("TotalMessages = %d, want 2 (削除済み3件を除く)", s.TotalMessages)
	}
	if s.MessagesToday != 2 {
		t.Errorf("MessagesToday = %d, want 2 (削除済み3件を除く)", s.MessagesToday)
	}
	// 削除済みしか無いコミュニティは「活動している」とは数えない。
	if s.ActiveCommunitiesLast30Days != 1 {
		t.Errorf("ActiveCommunitiesLast30Days = %d, want 1 (削除済みしか無いコミュニティを除く)", s.ActiveCommunitiesLast30Days)
	}

	// コミュニティ別のメッセージ数も同じ扱い。件数が 0 になっても
	// 一覧から消えてはいけない（LEFT JOIN の ON 句で除外している理由）。
	items, _, err := repo.GetCommunityAnalytics(context.Background(), repository.PageQuery{Limit: 10, WithTotal: true})
	if err != nil {
		t.Fatalf("GetCommunityAnalytics failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetCommunityAnalytics returned %d communities, want 2 (0件のコミュニティも残す)", len(items))
	}
	byName := map[string]int{}
	for _, it := range items {
		byName[it.Name] = it.MessageCount
	}
	if byName["live"] != 2 {
		t.Errorf("live community MessageCount = %d, want 2", byName["live"])
	}
	if byName["dead"] != 0 {
		t.Errorf("dead community MessageCount = %d, want 0", byName["dead"])
	}

	// 時系列の messages 系列も同じ。
	day := now.Format("2006-01-02")
	points, err := repo.GetTimeSeries(context.Background(), "day", day, day, repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	total := 0
	for _, p := range points {
		total += p.Messages
	}
	if total != 2 {
		t.Errorf("time series messages total = %d, want 2 (削除済み3件を除く)", total)
	}
}

// ■ 要求されていない集計SQLが実行されないこと
//
// 「選ばれた分だけ計算する」は、結果を見ても分からない（選ばれた分の値は前と
// 同じでなければならない）。分かるのは MySQL が受け取った SELECT の数だけなので、
// セッション状態のカウンタを前後で読んで差を取る。
func TestSummaryRunsOnlyRequestedQueries(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()
	singleConnDB(db)

	repo := NewMySQLAnalyticsRepository(db)
	ctx := context.Background()

	run := func(fields repository.FieldSet) int {
		var err error
		c := countStatements(t, db, func() {
			_, err = repo.GetAnalyticsSummary(ctx, fields)
		})
		if err != nil {
			t.Fatalf("GetAnalyticsSummary failed: %v", err)
		}
		return c.selects
	}

	all := run(repository.AllFields())
	if all < 30 {
		t.Fatalf("全部要求したときの SELECT が %d 本しかない。数え方が壊れている", all)
	}

	cases := []struct {
		name   string
		fields []string
		want   int
	}{
		// 1フィールド = 1本。
		{"単純な集計は1本", []string{"totalUsers"}, 1},
		{"論理削除つきでも1本", []string{"totalMessages"}, 1},
		// 派生値は元になる集計を引く。avgLikesPerPost は totalPosts と totalLikes。
		{"派生値は元データぶんだけ引く", []string{"avgLikesPerPost"}, 2},
		// dauMauRatio は dau と mau。
		{"比率も元データぶんだけ", []string{"dauMauRatio"}, 2},
		// 同じ元データを共有する派生値をまとめて選んでも、元データは1回。
		{"元データは共有される", []string{"avgLikesPerPost", "avgCommentsPerPost"}, 3},
		// avgCommunityMembers は totalCommunities と room_users の JOIN。
		{"コミュニティ平均は2本", []string{"avgCommunityMembers"}, 2},
		// セッション系はテーブルごとに分かれている。
		{"セッション統計は1本", []string{"avgSessionsPerDay"}, 1},
		{"画面別滞在時間は1本", []string{"pageViewStats"}, 1},
		// SQL を伴わない項目だけなら、DB には一切行かない。
		{"in-memory の項目だけなら SQL は 0 本", []string{"p95ResponseTimeMs", "webSocketConnections"}, 0},
		// 1つも選ばれていなければ 0 本。
		{"何も選ばれなければ 0 本", nil, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(repository.NewFieldSet(tc.fields)); got != tc.want {
				t.Fatalf("%v を要求したとき SELECT %d 本、期待 %d 本（全部なら %d 本）", tc.fields, got, tc.want, all)
			}
		})
	}
}

// 絞って計算しても、その項目の値は全部計算したときと同じであること。
func TestSummaryPartialMatchesFullValues(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	now := time.Now().Unix()
	mustExec(t, db, `INSERT INTO users (id, created_at, status, last_active_at) VALUES (1, ?, 'active', ?), (2, ?, 'frozen', ?)`, now, now, now, now)
	mustExec(t, db, `INSERT INTO posts (id, user_id, parent_id, created_at, deleted_at) VALUES
		(1, 1, NULL, ?, NULL), (2, 1, NULL, ?, NULL), (3, 1, 1, ?, NULL), (4, 2, NULL, ?, ?)`, now, now, now, now, now)
	mustExec(t, db, `INSERT INTO favorites (id, created_at) VALUES (1, ?), (2, ?), (3, ?)`, now, now, now)

	repo := NewMySQLAnalyticsRepository(db)
	ctx := context.Background()

	full, err := repo.GetAnalyticsSummary(ctx, repository.AllFields())
	if err != nil {
		t.Fatalf("GetAnalyticsSummary (all) failed: %v", err)
	}
	partial, err := repo.GetAnalyticsSummary(ctx, repository.NewFieldSet([]string{"totalPosts", "avgLikesPerPost"}))
	if err != nil {
		t.Fatalf("GetAnalyticsSummary (partial) failed: %v", err)
	}

	if partial.TotalPosts != full.TotalPosts {
		t.Errorf("TotalPosts = %d, want %d（全部計算したときと同じ）", partial.TotalPosts, full.TotalPosts)
	}
	if partial.AvgLikesPerPost != full.AvgLikesPerPost {
		t.Errorf("AvgLikesPerPost = %v, want %v（全部計算したときと同じ）", partial.AvgLikesPerPost, full.AvgLikesPerPost)
	}
	// 要求していないものはゼロ値のまま（呼び出し側は応答に出さない）。
	if partial.TotalUsers != 0 {
		t.Errorf("TotalUsers = %d, want 0（要求していないので引かない）", partial.TotalUsers)
	}
}

// 時系列も同じ。要求された系列ぶんしか GROUP BY を投げない。
func TestTimeSeriesRunsOnlyRequestedSeries(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()
	singleConnDB(db)

	repo := NewMySQLAnalyticsRepository(db)
	ctx := context.Background()
	day := time.Now().Format("2006-01-02")

	run := func(series repository.FieldSet) int {
		var err error
		c := countStatements(t, db, func() {
			_, err = repo.GetTimeSeries(ctx, "day", day, day, series)
		})
		if err != nil {
			t.Fatalf("GetTimeSeries failed: %v", err)
		}
		return c.selects
	}

	if got := run(repository.AllFields()); got != 6 {
		t.Fatalf("全系列で SELECT %d 本、期待 6 本", got)
	}
	if got := run(repository.NewFieldSet([]string{"label", "posts"})); got != 1 {
		t.Fatalf("posts だけで SELECT %d 本、期待 1 本（label は SQL を伴わない）", got)
	}
	if got := run(repository.NewFieldSet([]string{"label"})); got != 0 {
		t.Fatalf("label だけで SELECT %d 本、期待 0 本", got)
	}
	if got := run(repository.NewFieldSet([]string{"messages", "activeUsers"})); got != 2 {
		t.Fatalf("2系列で SELECT %d 本、期待 2 本", got)
	}

	// label は系列を選んでいなくても必ず埋まる。
	points, err := repo.GetTimeSeries(ctx, "day", day, day, repository.NewFieldSet([]string{"label"}))
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if len(points) != 1 || points[0].Label != day {
		t.Fatalf("points = %+v, want 1 点で label=%s", points, day)
	}
}
