package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"golang.org/x/sync/errgroup"
)

// analyticsQueryConcurrency bounds how many analytics queries run at once against
// the shared *sql.DB pool, so a single dashboard load can't starve other traffic.
const analyticsQueryConcurrency = 10

// ■ 論理削除の扱い（このファイル全体の約束）
//
// 論理削除の列（deleted_at）を持つのは posts と messages の2つだけ。集計は
// どちらも「生きている行だけ数える」で統一する。管理画面の数字は「いま場に
// 存在する投稿・メッセージの量」を表すものなので、削除済みを混ぜると
// 「消したのに減らない」数字になる。削除そのものを見たい枠は
// TotalDeletedPosts（deleted_at IS NOT NULL）として別に持っている。
//
// 以前は posts 系だけが deleted_at IS NULL で除外しており、messages 系は
// 除外していなかった。揃えたことで管理画面の次の数字が（削除済みメッセージの
// ぶんだけ）下がる:
//
//   - 総メッセージ数（TotalMessages）
//   - 本日のメッセージ数（MessagesToday）
//   - 直近30日のアクティブコミュニティ数（ActiveCommunitiesLast30Days）
//     … 削除済みメッセージしか無いコミュニティが落ちる
//   - 時系列グラフの messages 系列（GetTimeSeries）
//   - コミュニティ別のメッセージ数（GetCommunityAnalytics の MessageCount）
//   - ユニークDM送信者（UniqueDMSenders）… 下の定数で先に揃えてある
//
// messages に列を足したときと、messages を読む集計を足すときは、必ず
// deleted_at IS NULL を入れること（JOIN 側なら ON 句に入れる。WHERE へ書くと
// LEFT JOIN が INNER JOIN に化けて、メッセージ 0 件のコミュニティが一覧から
// 消える）。

// uniqueDMSendersQuery は「DM を送ったことのある実ユーザー数」。
//
// 以前はここが `SELECT COUNT(DISTINCT user_id) FROM messages` で、コミュニティ・
// 授業チャットの発言者も、論理削除済みメッセージしか持たないユーザーも数えていた
// （名前と実態が食い違っていた）。現在は「rooms.type = 'dm' のルームに、削除されて
// いないメッセージを1件以上投稿したユーザー」を数える。他の集計（TotalPosts など）
// と同じく、論理削除は deleted_at IS NULL で除外する。
// この修正で管理画面の「ユニークDM送信者」は従来より小さい値になる。
//
// 定数に切り出してあるのは、統合テスト（analytics_repository_test.go）から
// 実際に走る SQL そのものを検証するため。
const uniqueDMSendersQuery = `
	SELECT COUNT(DISTINCT m.user_id)
	FROM messages m
	INNER JOIN rooms r ON r.id = m.room_id
	WHERE r.type = ? AND m.deleted_at IS NULL`

type MySQLAnalyticsRepository struct {
	DB *sql.DB
}

func NewMySQLAnalyticsRepository(db *sql.DB) *MySQLAnalyticsRepository {
	return &MySQLAnalyticsRepository{DB: db}
}

// ■ フィールド名と集計の対応（ここが唯一の置き場）
//
// GraphQL で一部のフィールドしか選ばれていなくても、以前はサマリー1回で 35 本
// 以上の SQL を全部走らせていた。いまは「要求されたフィールドを養う集計だけ」
// 実行する。
//
// 対応づけは下の intQueries の serves と、その下の派生値の分岐だけに置く。
// リゾルバ・ユースケース側はフィールド名を集めて渡すだけで、どの名前が
// どの SQL を要するかは一切知らない（知ると、フィールドを足したときに
// 複数箇所を直す羽目になり、片方だけ直し忘れて「選んだのに 0 が返る」になる）。
//
// 平均・比率のような派生値は、元になる集計の serves に自分の名前を足して
// 表現する（例: avgLikesPerPost は totalLikes と totalPosts の両方に載る）。

// 複数の場所から参照するフィールド名だけ定数にしてある。1箇所しか出てこない
// 名前は文字列のまま（スキーマとの食い違いは TestSummaryFieldNamesMatchSchema
// が拾う）。定数にしているのは、serves と派生値の分岐の2箇所に同じ名前が
// 現れるもの＝書き間違えても気づけないものだけ。
const (
	fieldTotalUsers                = "totalUsers"
	fieldTotalPosts                = "totalPosts"
	fieldTotalComments             = "totalComments"
	fieldTotalLikes                = "totalLikes"
	fieldTotalCommunities          = "totalCommunities"
	fieldTotalFollows              = "totalFollows"
	fieldTotalNotifications        = "totalNotifications"
	fieldReadNotifications         = "readNotifications"
	fieldDAU                       = "dau"
	fieldMAU                       = "mau"
	fieldAvgLikesPerPost           = "avgLikesPerPost"
	fieldAvgCommentsPerPost        = "avgCommentsPerPost"
	fieldNotificationReadRate      = "notificationReadRate"
	fieldDAUMAURatio               = "dauMauRatio"
	fieldAvgCommunityMembers       = "avgCommunityMembers"
	fieldAvgCommunitiesPerUser     = "avgCommunitiesPerUser"
	fieldAvgFollowersPerUser       = "avgFollowersPerUser"
	fieldAvgFollowingPerUser       = "avgFollowingPerUser"
	fieldOnboardingCompleteRate    = "onboardingCompleteRate"
	fieldAvgTimeToFirstPostMinutes = "avgTimeToFirstPostMinutes"
	fieldAvgSessionDurationSeconds = "avgSessionDurationSeconds"
	fieldAvgSessionsPerDay         = "avgSessionsPerDay"
	fieldAvgScrollDepth            = "avgScrollDepth"
	fieldPageViewStats             = "pageViewStats"
)

// summaryNoSQLFields は SQL を伴わないフィールド（プロセスのメモリ上の値）。
//
// 選ばれていてもいなくても metrics.ApplyToSummary で必ず埋める。SQL が減る
// わけではないので絞る意味が無く、絞ると「キャッシュヒット時だけ最新」という
// 不揃いを作ってしまう。
var summaryNoSQLFields = []string{
	"webSocketConnections",
	"sseConnections",
	"errorRate5xx",
	"p50ResponseTimeMs",
	"p95ResponseTimeMs",
	"p99ResponseTimeMs",
}

// intQuery は1本の COUNT 系集計。
type intQuery struct {
	// serves はこの1本が養う GraphQL フィールド名。どれか1つでも要求されて
	// いれば実行する。
	serves []string
	dest   *int
	query  string
	args   []any
}

// summaryFieldNames はこのリポジトリが埋められる全フィールド名。
// スキーマの AnalyticsSummary と一致していることをテストで縛る。
func summaryFieldNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(names ...string) {
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	for _, q := range (&MySQLAnalyticsRepository{}).summaryIntQueries(&model.AnalyticsSummary{}, summaryWindows{}) {
		add(q.serves...)
	}
	add(fieldAvgCommunityMembers, fieldAvgCommunitiesPerUser, fieldOnboardingCompleteRate, fieldAvgTimeToFirstPostMinutes)
	add(fieldAvgSessionDurationSeconds, fieldAvgSessionsPerDay, fieldAvgScrollDepth, fieldPageViewStats)
	add(summaryNoSQLFields...)
	return out
}

// SummaryFieldNames は summaryFieldNames の公開版（スキーマとの突き合わせ用）。
func SummaryFieldNames() []string { return summaryFieldNames() }

// summaryWindows は集計の時間窓。intQueries の args に散らばると、
// 「今日」の定義が1つずれても気づけないので1つの値にまとめてある。
type summaryWindows struct {
	todayStart  int64
	weekStart   int64
	monthStart  int64
	days30Start int64
	active3Days int64
}

func (r *MySQLAnalyticsRepository) summaryIntQueries(s *model.AnalyticsSummary, w summaryWindows) []intQuery {
	return []intQuery{
		{[]string{fieldTotalUsers, fieldAvgCommunitiesPerUser, fieldAvgFollowersPerUser, fieldAvgFollowingPerUser, fieldOnboardingCompleteRate},
			&s.TotalUsers, `SELECT COUNT(*) FROM users`, nil},
		{[]string{"newUsersToday"}, &s.NewUsersToday, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{w.todayStart}},
		{[]string{"newUsersThisWeek"}, &s.NewUsersThisWeek, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{w.weekStart}},
		{[]string{"newUsersThisMonth"}, &s.NewUsersThisMonth, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{w.monthStart}},
		{[]string{"frozenUsersCount"}, &s.FrozenUsersCount, `SELECT COUNT(*) FROM users WHERE status = 'frozen'`, nil},
		{[]string{fieldTotalPosts, fieldAvgLikesPerPost, fieldAvgCommentsPerPost},
			&s.TotalPosts, `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL`, nil},
		{[]string{fieldTotalComments, fieldAvgCommentsPerPost},
			&s.TotalComments, `SELECT COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL`, nil},
		{[]string{"totalDeletedPosts"}, &s.TotalDeletedPosts, `SELECT COUNT(*) FROM posts WHERE deleted_at IS NOT NULL`, nil},
		{[]string{fieldTotalLikes, fieldAvgLikesPerPost}, &s.TotalLikes, `SELECT COUNT(*) FROM favorites`, nil},
		{[]string{fieldTotalCommunities, fieldAvgCommunityMembers}, &s.TotalCommunities, `SELECT COUNT(*) FROM communities`, nil},
		{[]string{"totalMessages"}, &s.TotalMessages, `SELECT COUNT(*) FROM messages WHERE deleted_at IS NULL`, nil},
		{[]string{"totalReports"}, &s.TotalReports, `SELECT COUNT(*) FROM user_reports`, nil},
		{[]string{"totalBlocks"}, &s.TotalBlocks, `SELECT COUNT(*) FROM blocks`, nil},
		{[]string{"totalInquiries"}, &s.TotalInquiries, `SELECT COUNT(*) FROM inquiries`, nil},
		{[]string{"currentActiveUsers"}, &s.CurrentActiveUsers, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{w.active3Days}},
		{[]string{fieldDAU, fieldDAUMAURatio}, &s.DAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{w.todayStart}},
		{[]string{"wau"}, &s.WAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{w.weekStart}},
		{[]string{fieldMAU, fieldDAUMAURatio}, &s.MAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{w.monthStart}},
		{[]string{"postsToday"}, &s.PostsToday, `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL AND created_at >= ?`, []any{w.todayStart}},
		{[]string{"commentsToday"}, &s.CommentsToday, `SELECT COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL AND created_at >= ?`, []any{w.todayStart}},
		{[]string{"messagesToday"}, &s.MessagesToday, `SELECT COUNT(*) FROM messages WHERE deleted_at IS NULL AND created_at >= ?`, []any{w.todayStart}},
		{[]string{"activeCommunitiesLast30Days"}, &s.ActiveCommunitiesLast30Days, `
			SELECT COUNT(DISTINCT c.id) FROM communities c
			INNER JOIN rooms r ON r.id = c.room_id
			INNER JOIN messages m ON m.room_id = r.id
			WHERE m.deleted_at IS NULL AND m.created_at >= ?`, []any{w.days30Start}},
		{[]string{fieldTotalFollows, fieldAvgFollowersPerUser, fieldAvgFollowingPerUser},
			&s.TotalFollows, `SELECT COUNT(*) FROM favorite_users`, nil},
		{[]string{"usersWithProfile"}, &s.UsersWithProfile, `SELECT COUNT(DISTINCT user_id) FROM profiles`, nil},
		{[]string{"usersWithAvatar"}, &s.UsersWithAvatar, `SELECT COUNT(*) FROM profiles WHERE avatar_media_id IS NOT NULL`, nil},
		{[]string{"usersWithPost"}, &s.UsersWithPost, `SELECT COUNT(DISTINCT user_id) FROM posts WHERE deleted_at IS NULL`, nil},
		{[]string{fieldTotalNotifications, fieldNotificationReadRate}, &s.TotalNotifications, `SELECT COUNT(*) FROM notifications`, nil},
		{[]string{fieldReadNotifications, fieldNotificationReadRate}, &s.ReadNotifications, `SELECT COUNT(*) FROM notifications WHERE is_read = TRUE`, nil},
		{[]string{"pendingReports"}, &s.PendingReports, `SELECT COUNT(*) FROM user_reports WHERE status = 'pending'`, nil},
		{[]string{"resolvedReports"}, &s.ResolvedReports, `SELECT COUNT(*) FROM user_reports WHERE status != 'pending'`, nil},
		{[]string{"uniqueDMSenders"}, &s.UniqueDMSenders, uniqueDMSendersQuery, []any{model.RoomTypeDM}},
		// テキストのみの投稿（post_mediaに紐付きなし）
		{[]string{"postsTextOnly"}, &s.PostsTextOnly, `
			SELECT COUNT(*) FROM posts p
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM post_media pm WHERE pm.post_id = p.id)`, nil},
		// 画像付き投稿
		{[]string{"postsWithImage"}, &s.PostsWithImage, `
			SELECT COUNT(DISTINCT p.id) FROM posts p
			INNER JOIN post_media pm ON pm.post_id = p.id
			INNER JOIN media m ON m.id = pm.media_id
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND m.content_type LIKE 'image/%'`, nil},
		// 動画付き投稿
		{[]string{"postsWithVideo"}, &s.PostsWithVideo, `
			SELECT COUNT(DISTINCT p.id) FROM posts p
			INNER JOIN post_media pm ON pm.post_id = p.id
			INNER JOIN media m ON m.id = pm.media_id
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND m.content_type LIKE 'video/%'`, nil},
	}
}

func (r *MySQLAnalyticsRepository) GetAnalyticsSummary(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	now := time.Now()
	w := summaryWindows{
		todayStart:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix(),
		weekStart:   now.AddDate(0, 0, -7).Unix(),
		monthStart:  now.AddDate(0, -1, 0).Unix(),
		days30Start: now.AddDate(0, 0, -30).Unix(),
		active3Days: now.AddDate(0, 0, -3).Unix(),
	}

	s := &model.AnalyticsSummary{}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(analyticsQueryConcurrency)

	for _, q := range r.summaryIntQueries(s, w) {
		if !fields.Wants(q.serves...) {
			continue
		}
		q := q
		g.Go(func() error {
			if q.args != nil {
				return r.DB.QueryRowContext(gctx, q.query, q.args...).Scan(q.dest)
			}
			return r.DB.QueryRowContext(gctx, q.query).Scan(q.dest)
		})
	}

	// コミュニティ平均メンバー数・ユーザーあたり平均参加数の元データ（同じ1本で両方まかなう）
	var totalCommunityMembers int
	if fields.Wants(fieldAvgCommunityMembers, fieldAvgCommunitiesPerUser) {
		g.Go(func() error {
			return r.DB.QueryRowContext(gctx, `
				SELECT COUNT(*) FROM room_users ru
				INNER JOIN communities c ON c.room_id = ru.room_id`).Scan(&totalCommunityMembers)
		})
	}

	// オンボーディング完了率（プロフィール + アバター + 初投稿）の元データ
	var onboardingComplete int
	if fields.Wants(fieldOnboardingCompleteRate) {
		g.Go(func() error {
			return r.DB.QueryRowContext(gctx, `
				SELECT COUNT(DISTINCT u.id) FROM users u
				INNER JOIN profiles p ON p.user_id = u.id AND p.avatar_media_id IS NOT NULL
				WHERE EXISTS (SELECT 1 FROM posts po WHERE po.user_id = u.id AND po.deleted_at IS NULL)`).Scan(&onboardingComplete)
		})
	}

	// 初投稿までの平均時間（分）
	var avgSeconds sql.NullFloat64
	if fields.Wants(fieldAvgTimeToFirstPostMinutes) {
		g.Go(func() error {
			return r.DB.QueryRowContext(gctx, `
				SELECT AVG(first_post.created_at - u.created_at) / 60.0
				FROM users u
				INNER JOIN (
					SELECT user_id, MIN(created_at) AS created_at
					FROM posts WHERE deleted_at IS NULL
					GROUP BY user_id
				) first_post ON first_post.user_id = u.id`).Scan(&avgSeconds)
		})
	}

	// セッションデータ（user_session_summaries テーブルが存在する場合のみ）。
	// テーブル未作成環境ではベストエフォートで無視するため、他クエリの失敗としては扱わない。
	if fields.Wants(fieldAvgSessionDurationSeconds, fieldAvgSessionsPerDay, fieldAvgScrollDepth, fieldPageViewStats) {
		g.Go(func() error {
			_ = r.loadSessionStats(gctx, s, fields)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 計算値。元になる集計を引いていないと 0 のまま割ることになるので、
	// 分岐の条件は上の serves と同じ名前で揃えること。
	if fields.Wants(fieldAvgLikesPerPost) && s.TotalPosts > 0 {
		s.AvgLikesPerPost = float64(s.TotalLikes) / float64(s.TotalPosts)
	}
	if fields.Wants(fieldAvgCommentsPerPost) && s.TotalPosts > 0 {
		s.AvgCommentsPerPost = float64(s.TotalComments) / float64(s.TotalPosts)
	}
	if fields.Wants(fieldNotificationReadRate) && s.TotalNotifications > 0 {
		s.NotificationReadRate = float64(s.ReadNotifications) / float64(s.TotalNotifications) * 100
	}
	// DAU/MAU 比。MAU が 0 のときは DAU も必ず 0（当日は当月に含まれる）なので、
	// ここだけ見れば足りる。以前は TotalUsers を分母にした式を先に代入し、直後に
	// この式で上書きしていた（前者は常に死んでいた）。
	if fields.Wants(fieldDAUMAURatio) && s.MAU > 0 {
		s.DAUMAURatio = float64(s.DAU) / float64(s.MAU) * 100
	}

	// コミュニティ平均メンバー数
	if fields.Wants(fieldAvgCommunityMembers) && s.TotalCommunities > 0 {
		s.AvgCommunityMembers = float64(totalCommunityMembers) / float64(s.TotalCommunities)
	}

	// ユーザーあたり平均コミュニティ参加数
	if fields.Wants(fieldAvgCommunitiesPerUser) && s.TotalUsers > 0 {
		s.AvgCommunitiesPerUser = float64(totalCommunityMembers) / float64(s.TotalUsers)
	}

	// フォロー/フォロワー平均
	if s.TotalUsers > 0 {
		if fields.Wants(fieldAvgFollowersPerUser) {
			s.AvgFollowersPerUser = float64(s.TotalFollows) / float64(s.TotalUsers)
		}
		if fields.Wants(fieldAvgFollowingPerUser) {
			s.AvgFollowingPerUser = float64(s.TotalFollows) / float64(s.TotalUsers)
		}
	}

	// オンボーディング完了率（プロフィール + アバター + 初投稿）
	if fields.Wants(fieldOnboardingCompleteRate) && s.TotalUsers > 0 {
		s.OnboardingCompleteRate = float64(onboardingComplete) / float64(s.TotalUsers) * 100
	}

	// 初投稿までの平均時間（分）
	if avgSeconds.Valid {
		s.AvgTimeToFirstPostMinutes = avgSeconds.Float64
	}

	// インフラ（in-memory）。SQL を伴わないので、選択されたフィールドに関係なく常に埋める。
	metrics.ApplyToSummary(s)

	return s, nil
}

func (r *MySQLAnalyticsRepository) loadSessionStats(ctx context.Context, s *model.AnalyticsSummary, fields repository.FieldSet) error {
	// 平均セッション時間（秒）
	if fields.Wants(fieldAvgSessionDurationSeconds) {
		var avgDuration sql.NullFloat64
		if err := r.DB.QueryRowContext(ctx, `
			SELECT AVG(total_duration_seconds / session_count)
			FROM user_session_summaries
			WHERE session_count > 0`).Scan(&avgDuration); err != nil {
			return err
		}
		if avgDuration.Valid {
			s.AvgSessionDurationSeconds = avgDuration.Float64
		}
	}

	// 1日あたりの平均セッション数
	if fields.Wants(fieldAvgSessionsPerDay) {
		var avgSessions sql.NullFloat64
		if err := r.DB.QueryRowContext(ctx, `
			SELECT AVG(session_count) FROM user_session_summaries`).Scan(&avgSessions); err != nil {
			return err
		}
		if avgSessions.Valid {
			s.AvgSessionsPerDay = avgSessions.Float64
		}
	}

	// 平均スクロール深度
	if fields.Wants(fieldAvgScrollDepth) {
		var avgScroll sql.NullFloat64
		if err := r.DB.QueryRowContext(ctx, `
			SELECT AVG(total_max_scroll_depth / view_count)
			FROM page_view_stats WHERE view_count > 0`).Scan(&avgScroll); err != nil {
			return err
		}
		if avgScroll.Valid {
			s.AvgScrollDepth = avgScroll.Float64
		}
	}

	if !fields.Wants(fieldPageViewStats) {
		return nil
	}

	// 画面別滞在時間（上位20ページ）
	rows, err := r.DB.QueryContext(ctx, `
		SELECT page_path,
		       AVG(total_duration_seconds / view_count) AS avg_duration,
		       AVG(total_max_scroll_depth / view_count) AS avg_scroll,
		       SUM(view_count) AS total_views
		FROM page_view_stats
		WHERE view_count > 0
		GROUP BY page_path
		ORDER BY total_views DESC
		LIMIT 20`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var stat model.PageViewStat
		if err := rows.Scan(&stat.PagePath, &stat.AvgDurationSeconds, &stat.AvgMaxScrollDepth, &stat.TotalViews); err != nil {
			return err
		}
		s.PageViewStats = append(s.PageViewStats, &stat)
	}
	return rows.Err()
}

func (r *MySQLAnalyticsRepository) GetCommunityAnalytics(ctx context.Context, q repository.PageQuery) ([]*model.CommunityStatItem, int, error) {
	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM communities`)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT c.id, c.name,
		       COUNT(DISTINCT ru.user_id) AS member_count,
		       COUNT(DISTINCT m.id) AS message_count
		FROM communities c
		LEFT JOIN room_users ru ON ru.room_id = c.room_id
		LEFT JOIN messages m ON m.room_id = c.room_id AND m.deleted_at IS NULL
		GROUP BY c.id, c.name
		ORDER BY member_count DESC
		LIMIT ? OFFSET ?`, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.CommunityStatItem
	for rows.Next() {
		item := &model.CommunityStatItem{}
		if err := rows.Scan(&item.CommunityID, &item.Name, &item.MemberCount, &item.MessageCount); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ■ 時系列の系列とフィールド名の対応（ここが唯一の置き場）
//
// TimeSeriesPoint は6系列あり、以前は選ばれていない系列ぶんも必ず 6 本の
// GROUP BY を投げていた。いまは要求された系列だけ引く。
// label は SQL を伴わない（範囲から作るラベル）ので常に埋まる。
const (
	seriesPosts       = "posts"
	seriesComments    = "comments"
	seriesMessages    = "messages"
	seriesNewUsers    = "newUsers"
	seriesLikes       = "likes"
	seriesActiveUsers = "activeUsers"
)

// TimeSeriesFieldNames は TimeSeriesPoint の全フィールド名（スキーマとの
// 突き合わせ用）。label を含む。
func TimeSeriesFieldNames() []string {
	return []string{"label", seriesPosts, seriesComments, seriesMessages, seriesNewUsers, seriesLikes, seriesActiveUsers}
}

// 時系列で使う時刻の書式。
//   - dateFmt      … 日次のラベル、および DATE 列（user_activity_dates.activity_date）へ渡す形
//   - hourLabelFmt … 時間別のラベル（GraphQL の応答に出る文字列）
//   - sqlDateTimeFmt … DATETIME 列（user_activity_hours.activity_hour）へ渡す形
const (
	dateFmt        = "2006-01-02"
	hourLabelFmt   = "2006-01-02 15:00"
	sqlDateTimeFmt = "2006-01-02 15:04:05"
)

func (r *MySQLAnalyticsRepository) GetTimeSeries(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	hourly := granularity == "hour"

	// サーバーが UTC コンテナで動いていても JST で統一する
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		jst = time.FixedZone("JST", 9*60*60)
	}
	now := time.Now().In(jst)

	fromT, err := time.ParseInLocation(dateFmt, from, jst)
	if err != nil {
		fromT = now.AddDate(0, 0, -30)
	}
	toT, err := time.ParseInLocation(dateFmt, to, jst)
	if err != nil {
		toT = now
	}
	// to は当日の終わりまでを含む
	toT = toT.AddDate(0, 0, 1)

	fromUnix := fromT.Unix()
	toUnix := toT.Unix()

	var labelFmt string
	if hourly {
		labelFmt = "%Y-%m-%d %H:00"
	} else {
		labelFmt = "%Y-%m-%d"
	}

	// MySQLのFROM_UNIXTIME()はMySQLサーバーのタイムゾーン（通常UTC）で変換するため
	// +32400秒（JST=UTC+9）を加算してラベルをJSTに揃える
	const jstOffsetSec = 9 * 60 * 60
	localTS := fmt.Sprintf("(created_at + %d)", jstOffsetSec)

	sinceExpr := fmt.Sprintf("%d", fromUnix)
	untilExpr := fmt.Sprintf("%d", toUnix)

	type metricQuery struct {
		dest  map[string]int
		query string
	}

	posts := map[string]int{}
	comments := map[string]int{}
	messages := map[string]int{}
	newUsers := map[string]int{}
	likes := map[string]int{}
	wantActive := series.Wants(seriesActiveUsers)

	// 範囲内の全スロットを生成してギャップを0で埋める。
	// 時間別の activeUsers がスロットの並びを先に要るのでここで作る。
	var labels []string
	if hourly {
		for t := fromT; t.Before(toT); t = t.Add(time.Hour) {
			labels = append(labels, t.Format(hourLabelFmt))
		}
	} else {
		for t := fromT; t.Before(toT); t = t.AddDate(0, 0, 1) {
			labels = append(labels, t.Format(dateFmt))
		}
	}

	// ■ activeUsers（そのスロットの時点から窓ぶん過去までに活動した実人数）
	//
	// 窓は日次が3日、時間別が72時間。単位は違うが数え方は同じなので、日次も時間別も
	// 同じ1本の経路（(user_id, スロット番号) を引いて rollingDistinctActiveUsers で
	// 窓を滑らせる）に通す。rollingDistinctActiveUsers はスロット番号を整数としか
	// 見ていないので、単位が「時」でも「日」でもそのまま使える。
	//
	// ■ 【重要】日次の数字は以前より小さくなる
	//
	// 以前の日次は「日別の DISTINCT 人数」を3日ぶん単純に足していた。窓が重なって
	// いるので、3日連続で活動した1人が3人として数えられていた（毎日来る常連ほど
	// 重複して効く）。つまり管理画面の日次グラフは実際より大きい値を出していた。
	// いまは窓の中で同じ user_id を1回しか数えないので、常連が多いほど下がる。
	// 上限は「実人数」で、下がったぶんが以前の数えすぎ。時間別のグラフは
	// もともとこの数え方なので変わらない。
	//
	// なお時間別が user_activity_hours を使うのは、users.last_active_at が
	// ユーザーごとに1値（最後の活動時刻）しか持たず、10時と14時に活動した人が
	// 14時のスロットにしか現れないため（過去の時間帯ほど実際より少なく出ていた）。
	const (
		activeUsersWindowHours = 72
		activeUsersWindowDays  = 3
	)
	// activeUsersQuery は (user_id, スロット番号) を user_id 昇順・スロット昇順で返す。
	// activeUsersWindow はそのスロット番号の単位で数えた窓の長さ。
	var activeUsersQuery string
	var activeUsersWindow int
	if hourly {
		activeUsersWindow = activeUsersWindowHours
		// 窓のぶん手前から引く。fromT のスロットは fromT-71h の活動まで数える。
		activeExtendedFrom := fromT.Add(-time.Duration(activeUsersWindowHours-1) * time.Hour)
		// activity_hour は JST の時の始まりをそのまま入れてある列なので、
		// created_at / last_active_at のような Unix 秒と違い jstOffsetSec の補正は要らない
		// （user_activity_dates.activity_date と同じ流儀）。
		// TIMESTAMPDIFF で最初のスロットからの経過時間＝スロット番号にして返す
		// （負なら範囲より手前の活動）。並びは主キー (user_id, activity_hour) の順。
		activeUsersQuery = fmt.Sprintf(
			`SELECT user_id, TIMESTAMPDIFF(HOUR, '%s', activity_hour) AS slot FROM user_activity_hours WHERE activity_hour >= '%s' AND activity_hour < '%s' ORDER BY user_id, activity_hour`,
			fromT.Format(sqlDateTimeFmt), activeExtendedFrom.Format(sqlDateTimeFmt), toT.Format(sqlDateTimeFmt))
	} else {
		activeUsersWindow = activeUsersWindowDays
		// 時間別と同じで窓のぶん手前から引く（fromT のスロットは2日前の活動まで数える）。
		activeExtendedFrom := fromT.AddDate(0, 0, -(activeUsersWindowDays - 1)).Format(dateFmt)
		activeExtendedTo := toT.AddDate(0, 0, -1).Format(dateFmt) // toT は翌日 00:00 なので1日戻す
		// DATEDIFF で最初のスロットからの経過日数＝スロット番号にする
		// （負なら範囲より手前の活動）。並びは主キー (user_id, activity_date) の順。
		activeUsersQuery = fmt.Sprintf(
			`SELECT user_id, DATEDIFF(activity_date, '%s') AS slot FROM user_activity_dates WHERE activity_date >= '%s' AND activity_date <= '%s' ORDER BY user_id, activity_date`,
			fromT.Format(dateFmt), activeExtendedFrom, activeExtendedTo)
	}

	between := fmt.Sprintf("created_at >= %s AND created_at < %s", sinceExpr, untilExpr)
	// 系列名 → 引く SQL の対応。要求された系列だけを g へ積む。
	all := []struct {
		field string
		mq    metricQuery
	}{
		{seriesPosts, metricQuery{posts, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL AND %s GROUP BY lbl`, localTS, labelFmt, between)}},
		{seriesComments, metricQuery{comments, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL AND %s GROUP BY lbl`, localTS, labelFmt, between)}},
		{seriesMessages, metricQuery{messages, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM messages WHERE deleted_at IS NULL AND %s GROUP BY lbl`, localTS, labelFmt, between)}},
		{seriesNewUsers, metricQuery{newUsers, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM users WHERE %s GROUP BY lbl`, localTS, labelFmt, between)}},
		{seriesLikes, metricQuery{likes, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM favorites WHERE %s GROUP BY lbl`, localTS, labelFmt, between)}},
	}

	// activeUsers はここに並べていない。他の系列は「ラベル → 件数」で引けるが、
	// activeUsers は窓の中の重複を潰すのに user_id が要るので形が違う。下で別に投げる。
	mqs := make([]metricQuery, 0, len(all))
	for _, e := range all {
		if !series.Wants(e.field) {
			continue
		}
		mqs = append(mqs, e.mq)
	}

	activeUsers := make(map[string]int, len(labels))

	g, gctx := errgroup.WithContext(ctx)
	for _, mq := range mqs {
		mq := mq
		g.Go(func() error {
			rows, err := r.DB.QueryContext(gctx, mq.query)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var lbl string
				var cnt int
				if err := rows.Scan(&lbl, &cnt); err != nil {
					return err
				}
				mq.dest[lbl] = cnt
			}
			return rows.Err()
		})
	}
	// activeUsers（窓の中の実人数）。日次・時間別とも並行に引く1本で、
	// 結果は labels の並びのまま返ってくる。
	if wantActive {
		g.Go(func() error {
			counts, err := rollingDistinctActiveUsers(gctx, r.DB, activeUsersQuery, len(labels), activeUsersWindow)
			if err != nil {
				return err
			}
			for i, l := range labels {
				activeUsers[l] = counts[i]
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	points := make([]*model.TimeSeriesPoint, len(labels))
	for i, l := range labels {
		points[i] = &model.TimeSeriesPoint{
			Label:       l,
			Posts:       posts[l],
			Comments:    comments[l],
			Messages:    messages[l],
			NewUsers:    newUsers[l],
			Likes:       likes[l],
			ActiveUsers: activeUsers[l],
		}
	}
	return points, nil
}

// rollingDistinctActiveUsers は「各スロットの時点から window スロットぶん過去までに
// 活動した実人数」をスロット順に返す。日次（1スロット＝1日・窓3）と時間別
// （1スロット＝1時間・窓72）の両方がこれを使う。スロット番号は整数としか見て
// いないので、単位はこの関数の外で決まる。
//
// query は (user_id, スロット番号) を user_id 昇順・スロット番号昇順で返すこと。
// スロット番号は最初のスロットからの経過スロット数で、範囲より手前の活動は負になる。
//
// ■ なぜスロットごとに COUNT(DISTINCT user_id) を足すのでは駄目か
// 窓は隣り合うスロットで重なっているので、同じ人が窓の中の複数のスロットに現れる。
// 足すとその人をスロットの数だけ数えてしまう（時間別なら毎時アクセスする1人が
// 72人ぶん、日次なら3日連続で来た1人が3人ぶん）。窓ごとに DISTINCT を取り直すのが
// 正しいが、SQL でスロットの数だけ窓を数え直すと重い。
//
// ■ 代わりにやっていること
// 1件の活動（スロット a）が人数に効くのは出力スロット a..a+window-1 の区間。
// 同じ人の区間を重ねて1本に潰し（だから窓の中で二重に数えない）、区間の始まりで
// +1・終わりの次で -1 を置いて最後に累積する（いもす法）。
// user_id 順に並んでいるので、人ごとの区間は流しながら潰せる。
// 持つのはスロット数ぶんの配列だけで、活動の件数に比例したメモリは要らない。
//
// ■ 引く行数
// （窓を含む範囲で活動した人数）×（その人が活動した時間帯の数）。管理画面の
// プリセットで最大の「過去90日 × 時間別」がいちばん大きく、DAU 1,000 人・
// 1人1日4時間帯なら 36万行ほど。行はスカラー2つなので流す間のメモリは増えないが、
// ここが重くなったら「時間別で選べる範囲を絞る」のが先（表の持ち方を変えても
// 読む行数は変わらない）。
func rollingDistinctActiveUsers(ctx context.Context, db *sql.DB, query string, slots, window int) ([]int, error) {
	counts := make([]int, slots)
	if slots <= 0 {
		return counts, nil
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	diff := make([]int, slots+1)
	var (
		curUser  int64
		haveUser bool
		start    int
		end      int
		haveSpan bool
	)
	flush := func() {
		if haveSpan {
			diff[start]++
			diff[end+1]--
			haveSpan = false
		}
	}

	for rows.Next() {
		var userID int64
		var slot int
		if err := rows.Scan(&userID, &slot); err != nil {
			return nil, err
		}
		if !haveUser || userID != curUser {
			flush()
			curUser, haveUser = userID, true
		}

		// この活動が覆う出力スロットの範囲（範囲外は切り詰める）。
		spanStart, spanEnd := slot, slot+window-1
		if spanStart < 0 {
			spanStart = 0
		}
		if spanEnd > slots-1 {
			spanEnd = slots - 1
		}
		if spanStart > spanEnd {
			continue // 窓が範囲に一切かからない（起こらないはずだが、引き方が変わっても壊れないように）
		}

		// 同じ人の隣接・重複する区間は1本に潰す。スロット番号昇順なので前とだけ見ればよい。
		if haveSpan && spanStart <= end+1 {
			if spanEnd > end {
				end = spanEnd
			}
			continue
		}
		flush()
		start, end, haveSpan = spanStart, spanEnd, true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	flush()

	running := 0
	for i := 0; i < slots; i++ {
		running += diff[i]
		counts[i] = running
	}
	return counts, nil
}
