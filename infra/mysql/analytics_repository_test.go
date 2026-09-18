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
		// db/migrations/070 から外部キー無し・索引だけ落とした形。主キーは残す
		// （同じ人・同じ時間帯が1行なのは集計の前提なので、テストでも効かせる）。
		`CREATE TABLE user_activity_hours (
			user_id BIGINT NOT NULL,
			activity_hour DATETIME NOT NULL,
			PRIMARY KEY (user_id, activity_hour)
		)`,
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

// ■ 時間別 activeUsers（ここが今回の本題）
//
// 直したいのは「同じ人が複数の時間帯に活動しても、最後の1スロットにしか出ない」こと。
// 原因は users.last_active_at がユーザーごとに1値しか持たないこと。時間解像度の
// 活動履歴（user_activity_hours）から数えれば、活動した時間帯すべてに出る。
//
// 集計の窓は従来どおり72時間のローリング（あるスロットの値は、そのスロットの時点から
// 72時間さかのぼる間に活動した実人数）。窓は隣のスロットと重なるので、窓の中で
// 同じ人を二重に数えないことも同時に確かめる。

// jstHourUnix は JST の時刻を Unix 秒にする（last_active_at 用）。
func jstHourUnix(t *testing.T, s string) int64 {
	t.Helper()
	loc := time.FixedZone("JST", 9*60*60)
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	if err != nil {
		t.Fatalf("failed to parse the JST time %q: %v", s, err)
	}
	return ts.Unix()
}

func TestTimeSeriesHourlyActiveUsersCountsEveryHour(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	const (
		alice = int64(1) // 10時と14時に活動（本題: 10時のスロットにも出るべき）
		bob   = int64(2) // 10・11・12時に活動（窓の中で3人ぶんに数えてはいけない）
		carol = int64(3) // 前日23時（範囲の手前だが72時間の窓には入る）
		dave  = int64(4) // 4日前（どの窓にも入らない）
	)

	// last_active_at は「最後の活動時刻」だけを持つ。修正前の集計はこれを見ていた。
	mustExec(t, db, `INSERT INTO users (id, created_at, status, last_active_at) VALUES (?,?,'active',?),(?,?,'active',?),(?,?,'active',?),(?,?,'active',?)`,
		alice, jstHourUnix(t, "2026-09-01 00:00:00"), jstHourUnix(t, "2026-09-18 14:30:00"),
		bob, jstHourUnix(t, "2026-09-01 00:00:00"), jstHourUnix(t, "2026-09-18 12:30:00"),
		carol, jstHourUnix(t, "2026-09-01 00:00:00"), jstHourUnix(t, "2026-09-17 23:30:00"),
		dave, jstHourUnix(t, "2026-09-01 00:00:00"), jstHourUnix(t, "2026-09-14 12:30:00"),
	)
	mustExec(t, db, `INSERT INTO user_activity_hours (user_id, activity_hour) VALUES
		(?, '2026-09-18 10:00:00'),
		(?, '2026-09-18 14:00:00'),
		(?, '2026-09-18 10:00:00'),
		(?, '2026-09-18 11:00:00'),
		(?, '2026-09-18 12:00:00'),
		(?, '2026-09-17 23:00:00'),
		(?, '2026-09-14 12:00:00')`,
		alice, alice, bob, bob, bob, carol, dave)

	repo := NewMySQLAnalyticsRepository(db)
	points, err := repo.GetTimeSeries(context.Background(), "hour", "2026-09-18", "2026-09-18", repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if len(points) != 24 {
		t.Fatalf("points = %d, want 24 (1日ぶんの時間スロット)", len(points))
	}

	got := map[string]int{}
	for _, p := range points {
		got[p.Label] = p.ActiveUsers
	}

	// 00:00〜09:00 は carol だけ（前日23時の活動が72時間の窓に入っている）。
	// 10:00 以降は alice と bob が加わって3人。dave は窓の外なのでどこにも出ない。
	for h := 0; h < 24; h++ {
		label := fmt.Sprintf("2026-09-18 %02d:00", h)
		want := 1
		if h >= 10 {
			want = 3
		}
		if got[label] != want {
			t.Errorf("activeUsers[%s] = %d, want %d", label, got[label], want)
		}
	}

	// 本題の確認をもう一度、狙いを名指しで。
	// 10時のスロットに alice が出ること（修正前は最後の活動である14時にしか出なかった）。
	if got["2026-09-18 10:00"] != 3 {
		t.Errorf("10時のスロット = %d, want 3（10時に活動した alice と bob が出ること）", got["2026-09-18 10:00"])
	}
	// 12時のスロットで bob が3人ぶんに膨らんでいないこと（窓の中の重複を潰す）。
	if got["2026-09-18 12:00"] != 3 {
		t.Errorf("12時のスロット = %d, want 3（3時間帯に活動した bob は1人）", got["2026-09-18 12:00"])
	}

	// 修正前の集計が何を見ていたか（＝なぜ直せなかったか）を固定しておく。
	// 10時台に last_active_at を持つユーザーは0人。alice も bob もその後さらに
	// 活動しているので、last_active_at からは「10時に活動した」ことが分からない。
	var lastActiveInHour10 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE DATE_FORMAT(FROM_UNIXTIME(last_active_at + 32400), '%Y-%m-%d %H:00') = '2026-09-18 10:00'`).Scan(&lastActiveInHour10); err != nil {
		t.Fatalf("failed to run the legacy count: %v", err)
	}
	if lastActiveInHour10 != 0 {
		t.Fatalf("legacy count = %d, want 0; the fixture no longer demonstrates the bug", lastActiveInHour10)
	}
}

// 日次の activeUsers は user_activity_dates だけから決まること。
// 時間別の表（user_activity_hours）に何が入っていても日次は動かない。
func TestTimeSeriesDailyActiveUsersUnchangedByTheHourlyTable(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	const (
		alice    = int64(1)
		bob      = int64(2)
		hourOnly = int64(99) // 時間別の表にしか行が無い人。日次に混ざってはいけない。
	)
	mustExec(t, db, `INSERT INTO user_activity_dates (user_id, activity_date) VALUES
		(?, '2026-09-16'), (?, '2026-09-17'), (?, '2026-09-18'), (?, '2026-09-18')`,
		alice, alice, alice, bob)

	repo := NewMySQLAnalyticsRepository(db)
	daily := func() int {
		points, err := repo.GetTimeSeries(context.Background(), "day", "2026-09-18", "2026-09-18", repository.AllFields())
		if err != nil {
			t.Fatalf("GetTimeSeries failed: %v", err)
		}
		if len(points) != 1 {
			t.Fatalf("points = %d, want 1", len(points))
		}
		return points[0].ActiveUsers
	}

	// 日次は3日ぶんのローリング窓の「実人数」。窓には alice（9/16・17・18）と
	// bob（9/18）が入るので2人。
	//
	// 以前はここが 4 だった（日別の DISTINCT 人数 1+1+2 を単純に足していたため、
	// 3日とも来た alice が3人ぶんに膨らんでいた）。
	before := daily()
	if before != 2 {
		t.Fatalf("daily activeUsers = %d, want 2 (窓に居るのは alice と bob の2人)", before)
	}

	// 時間別の表を埋めても日次は動かない（別の人を入れても増えない）。
	mustExec(t, db, `INSERT INTO user_activity_hours (user_id, activity_hour) VALUES
		(?, '2026-09-18 10:00:00'),
		(?, '2026-09-18 11:00:00'),
		(?, '2026-09-18 10:00:00')`,
		alice, alice, hourOnly)

	if after := daily(); after != before {
		t.Fatalf("daily activeUsers changed after filling user_activity_hours: %d -> %d", before, after)
	}
}

// 時間別でも「要求された系列ぶんしか SQL を投げない」が保たれること。
// activeUsers の引き方を変えたので、本数が増えていないことを固定しておく。
func TestTimeSeriesHourlyRunsOnlyRequestedSeries(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()
	singleConnDB(db)

	repo := NewMySQLAnalyticsRepository(db)
	ctx := context.Background()

	run := func(series repository.FieldSet) int {
		var err error
		c := countStatements(t, db, func() {
			_, err = repo.GetTimeSeries(ctx, "hour", "2026-09-18", "2026-09-18", series)
		})
		if err != nil {
			t.Fatalf("GetTimeSeries failed: %v", err)
		}
		return c.selects
	}

	if got := run(repository.AllFields()); got != 6 {
		t.Fatalf("全系列で SELECT %d 本、期待 6 本", got)
	}
	if got := run(repository.NewFieldSet([]string{"label", "activeUsers"})); got != 1 {
		t.Fatalf("activeUsers だけで SELECT %d 本、期待 1 本", got)
	}
	if got := run(repository.NewFieldSet([]string{"label", "posts"})); got != 1 {
		t.Fatalf("posts だけで SELECT %d 本、期待 1 本", got)
	}
	if got := run(repository.NewFieldSet([]string{"label"})); got != 0 {
		t.Fatalf("label だけで SELECT %d 本、期待 0 本", got)
	}
}

// migration 070 を実際に流して、043 の既存データが壊れないこと。
//
// 案として「user_activity_dates に時刻の列を足して主キーを広げる」もあったが、
// 本番適用済みの表を作り替える必要があった。採ったのは表を足すほうなので、
// ここで確かめるのは「既存の表と、そこから出る日次の数字に一切触っていない」こと。
func TestMigration070LeavesUserActivityDatesIntact(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_migration070_test", nil)
	defer cleanup()

	ctx := context.Background()
	runMigration := func(name string) {
		t.Helper()
		sqlBytes, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatalf("failed to read the migration %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("failed to run the migration %s: %v", name, err)
		}
	}

	// 043 の状態（本番にある形）を作って、既存データを入れる。
	runMigration("043_create_user_activity_dates.up.sql")
	mustExec(t, db, `INSERT INTO user_activity_dates (user_id, activity_date) VALUES (1, '2026-09-17'), (1, '2026-09-18'), (2, '2026-09-18')`)

	dailyCounts := func() map[string]int {
		t.Helper()
		rows, err := db.QueryContext(ctx, `SELECT activity_date, COUNT(DISTINCT user_id) FROM user_activity_dates GROUP BY activity_date`)
		if err != nil {
			t.Fatalf("failed to aggregate the activity dates: %v", err)
		}
		defer rows.Close()
		out := map[string]int{}
		for rows.Next() {
			var d string
			var c int
			if err := rows.Scan(&d, &c); err != nil {
				t.Fatalf("failed to scan: %v", err)
			}
			out[d[:10]] = c
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("failed to iterate: %v", err)
		}
		return out
	}
	before := dailyCounts()

	runMigration("070_create_user_activity_hours.up.sql")

	after := dailyCounts()
	if len(after) != len(before) || after["2026-09-17"] != before["2026-09-17"] || after["2026-09-18"] != before["2026-09-18"] {
		t.Fatalf("daily counts changed by the migration: %v -> %v", before, after)
	}
	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_dates`).Scan(&rowCount); err != nil {
		t.Fatalf("failed to count the existing rows: %v", err)
	}
	if rowCount != 3 {
		t.Fatalf("user_activity_dates has %d rows, want the original 3", rowCount)
	}

	// 新しい表は「同じ人・同じ時間帯は1行」。書き込み側は INSERT IGNORE なので、
	// 二重に投げても増えないことが前提になっている。
	mustExec(t, db, `INSERT IGNORE INTO user_activity_hours (user_id, activity_hour) VALUES (1, '2026-09-18 10:00:00')`)
	mustExec(t, db, `INSERT IGNORE INTO user_activity_hours (user_id, activity_hour) VALUES (1, '2026-09-18 10:00:00')`)
	var hourRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_hours`).Scan(&hourRows); err != nil {
		t.Fatalf("failed to count the hourly rows: %v", err)
	}
	if hourRows != 1 {
		t.Fatalf("user_activity_hours has %d rows, want 1 (同じ時間帯の2回目は捨てる)", hourRows)
	}

	// down を流しても、既存の表は残る（触っていないので当たり前だが、
	// 巻き戻しが日次を壊さないことをここで固定する）。
	runMigration("070_create_user_activity_hours.down.sql")
	if got := dailyCounts(); len(got) != len(before) || got["2026-09-18"] != before["2026-09-18"] {
		t.Fatalf("daily counts changed by the rollback: %v -> %v", before, got)
	}
}

// ■ 日次 activeUsers の重複カウント（今回直したところ）
//
// 日次も時間別と同じ「3日の窓に居る実人数」を出す。以前は日別の DISTINCT 人数を
// 3日ぶん単純に足していたので、3日連続で活動した1人が3人として数えられていた。
// 管理画面の日次グラフはこの修正で下がる（常連が多いほど下がり幅が大きい）。

// dailyActiveUsers は1日ぶんの日次グラフを引いて activeUsers を返す。
func dailyActiveUsers(t *testing.T, db *sql.DB, day string) int {
	t.Helper()
	points, err := NewMySQLAnalyticsRepository(db).GetTimeSeries(
		context.Background(), "day", day, day, repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	return points[0].ActiveUsers
}

// 3日連続で活動した1人が「3」ではなく「1」と数えられること。
func TestTimeSeriesDailyActiveUsersCountsAPersonOnce(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	const regular = int64(1) // 9/16・17・18 と毎日活動する常連
	mustExec(t, db, `INSERT INTO user_activity_dates (user_id, activity_date) VALUES
		(?, '2026-09-16'), (?, '2026-09-17'), (?, '2026-09-18')`,
		regular, regular, regular)

	if got := dailyActiveUsers(t, db, "2026-09-18"); got != 1 {
		t.Fatalf("daily activeUsers = %d, want 1（3日とも来た1人を3人と数えないこと）", got)
	}
}

// 3日の窓の端の扱い。9/18 のスロットは 9/16 までを数え、9/15 は数えない。
func TestTimeSeriesDailyActiveUsersWindowEdges(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	const (
		onDay      = int64(1) // 9/18 … 窓の内側（当日）
		twoDaysAgo = int64(2) // 9/16 … 窓の内側（いちばん古い日）
		threeAgo   = int64(3) // 9/15 … 窓の外（1日ぶんはみ出す）
	)
	mustExec(t, db, `INSERT INTO user_activity_dates (user_id, activity_date) VALUES
		(?, '2026-09-18'), (?, '2026-09-16'), (?, '2026-09-15')`,
		onDay, twoDaysAgo, threeAgo)

	// 9/18 の窓は 9/16〜9/18。9/15 の人だけが外。
	if got := dailyActiveUsers(t, db, "2026-09-18"); got != 2 {
		t.Fatalf("2026-09-18 の activeUsers = %d, want 2（9/16 は窓の内、9/15 は外）", got)
	}

	// 9/17 の窓は 9/15〜9/17。当日（9/18）の人が外れ、9/15 の人が入る。
	if got := dailyActiveUsers(t, db, "2026-09-17"); got != 2 {
		t.Fatalf("2026-09-17 の activeUsers = %d, want 2（9/16 と 9/15 の2人）", got)
	}

	// 9/20 の窓は 9/18〜9/20。当日の人だけ残る。
	if got := dailyActiveUsers(t, db, "2026-09-20"); got != 1 {
		t.Fatalf("2026-09-20 の activeUsers = %d, want 1（9/18 の人だけ窓に残る）", got)
	}

	// 9/21 の窓は 9/19〜9/21。誰も居ない。
	if got := dailyActiveUsers(t, db, "2026-09-21"); got != 0 {
		t.Fatalf("2026-09-21 の activeUsers = %d, want 0（窓に誰も居ない）", got)
	}
}

// 複数日にまたがる範囲でも、各スロットが「その日の窓の実人数」になること。
// 範囲の手前（表示しない日）の活動も窓に入れて数えること。
func TestTimeSeriesDailyActiveUsersAcrossARange(t *testing.T) {
	db, cleanup := analyticsTestDB(t)
	defer cleanup()

	const (
		regular = int64(1) // 9/16〜9/18 毎日
		newbie  = int64(2) // 9/18 だけ
		early   = int64(3) // 9/15 だけ（範囲の手前。9/16・9/17 の窓には入る）
	)
	mustExec(t, db, `INSERT INTO user_activity_dates (user_id, activity_date) VALUES
		(?, '2026-09-16'), (?, '2026-09-17'), (?, '2026-09-18'),
		(?, '2026-09-18'),
		(?, '2026-09-15')`,
		regular, regular, regular, newbie, early)

	points, err := NewMySQLAnalyticsRepository(db).GetTimeSeries(
		context.Background(), "day", "2026-09-16", "2026-09-18", repository.AllFields())
	if err != nil {
		t.Fatalf("GetTimeSeries failed: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("points = %d, want 3", len(points))
	}

	// 9/16 の窓(9/14-16): early と regular → 2
	// 9/17 の窓(9/15-17): early と regular → 2
	// 9/18 の窓(9/16-18): regular と newbie → 2（regular を3回数えない）
	want := map[string]int{
		"2026-09-16": 2,
		"2026-09-17": 2,
		"2026-09-18": 2,
	}
	for _, p := range points {
		if want[p.Label] != p.ActiveUsers {
			t.Errorf("activeUsers[%s] = %d, want %d", p.Label, p.ActiveUsers, want[p.Label])
		}
	}
}
