package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	"github.com/Cityboypenguin/SPACE-server/model"
	"golang.org/x/sync/errgroup"
)

// analyticsQueryConcurrency bounds how many analytics queries run at once against
// the shared *sql.DB pool, so a single dashboard load can't starve other traffic.
const analyticsQueryConcurrency = 10

type MySQLAnalyticsRepository struct {
	DB *sql.DB
}

func NewMySQLAnalyticsRepository(db *sql.DB) *MySQLAnalyticsRepository {
	return &MySQLAnalyticsRepository{DB: db}
}

func (r *MySQLAnalyticsRepository) GetAnalyticsSummary(ctx context.Context) (*model.AnalyticsSummary, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	weekStart := now.AddDate(0, 0, -7).Unix()
	monthStart := now.AddDate(0, -1, 0).Unix()
	days30Start := now.AddDate(0, 0, -30).Unix()

	s := &model.AnalyticsSummary{}

	type intQuery struct {
		dest  *int
		query string
		args  []any
	}

	intQueries := []intQuery{
		{&s.TotalUsers, `SELECT COUNT(*) FROM users`, nil},
		{&s.NewUsersToday, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{todayStart}},
		{&s.NewUsersThisWeek, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{weekStart}},
		{&s.NewUsersThisMonth, `SELECT COUNT(*) FROM users WHERE created_at >= ?`, []any{monthStart}},
		{&s.FrozenUsersCount, `SELECT COUNT(*) FROM users WHERE status = 'frozen'`, nil},
		{&s.TotalPosts, `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL`, nil},
		{&s.TotalComments, `SELECT COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL`, nil},
		{&s.TotalDeletedPosts, `SELECT COUNT(*) FROM posts WHERE deleted_at IS NOT NULL`, nil},
		{&s.TotalLikes, `SELECT COUNT(*) FROM favorites`, nil},
		{&s.TotalCommunities, `SELECT COUNT(*) FROM communities`, nil},
		{&s.TotalMessages, `SELECT COUNT(*) FROM messages`, nil},
		{&s.TotalReports, `SELECT COUNT(*) FROM user_reports`, nil},
		{&s.TotalBlocks, `SELECT COUNT(*) FROM blocks`, nil},
		{&s.TotalInquiries, `SELECT COUNT(*) FROM inquiries`, nil},
		{&s.DAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{todayStart}},
		{&s.WAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{weekStart}},
		{&s.MAU, `SELECT COUNT(*) FROM users WHERE last_active_at >= ?`, []any{monthStart}},
		{&s.PostsToday, `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL AND created_at >= ?`, []any{todayStart}},
		{&s.CommentsToday, `SELECT COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL AND created_at >= ?`, []any{todayStart}},
		{&s.MessagesToday, `SELECT COUNT(*) FROM messages WHERE created_at >= ?`, []any{todayStart}},
		{&s.ActiveCommunitiesLast30Days, `
			SELECT COUNT(DISTINCT c.id) FROM communities c
			INNER JOIN rooms r ON r.id = c.room_id
			INNER JOIN messages m ON m.room_id = r.id
			WHERE m.created_at >= ?`, []any{days30Start}},
		{&s.TotalFollows, `SELECT COUNT(*) FROM favorite_users`, nil},
		{&s.UsersWithProfile, `SELECT COUNT(DISTINCT user_id) FROM profiles`, nil},
		{&s.UsersWithAvatar, `SELECT COUNT(*) FROM profiles WHERE avatar_media_id IS NOT NULL`, nil},
		{&s.UsersWithPost, `SELECT COUNT(DISTINCT user_id) FROM posts WHERE deleted_at IS NULL`, nil},
		{&s.TotalNotifications, `SELECT COUNT(*) FROM notifications`, nil},
		{&s.ReadNotifications, `SELECT COUNT(*) FROM notifications WHERE is_read = TRUE`, nil},
		{&s.PendingReports, `SELECT COUNT(*) FROM user_reports WHERE status = 'pending'`, nil},
		{&s.ResolvedReports, `SELECT COUNT(*) FROM user_reports WHERE status != 'pending'`, nil},
		{&s.UniqueDMSenders, `SELECT COUNT(DISTINCT user_id) FROM messages`, nil},
		// テキストのみの投稿（post_mediaに紐付きなし）
		{&s.PostsTextOnly, `
			SELECT COUNT(*) FROM posts p
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM post_media pm WHERE pm.post_id = p.id)`, nil},
		// 画像付き投稿
		{&s.PostsWithImage, `
			SELECT COUNT(DISTINCT p.id) FROM posts p
			INNER JOIN post_media pm ON pm.post_id = p.id
			INNER JOIN media m ON m.id = pm.media_id
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND m.content_type LIKE 'image/%'`, nil},
		// 動画付き投稿
		{&s.PostsWithVideo, `
			SELECT COUNT(DISTINCT p.id) FROM posts p
			INNER JOIN post_media pm ON pm.post_id = p.id
			INNER JOIN media m ON m.id = pm.media_id
			WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
			AND m.content_type LIKE 'video/%'`, nil},
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(analyticsQueryConcurrency)

	for _, q := range intQueries {
		q := q
		g.Go(func() error {
			if q.args != nil {
				return r.DB.QueryRowContext(gctx, q.query, q.args...).Scan(q.dest)
			}
			return r.DB.QueryRowContext(gctx, q.query).Scan(q.dest)
		})
	}

	// コミュニティ平均メンバー数の元データ
	var totalCommunityMembers int
	g.Go(func() error {
		return r.DB.QueryRowContext(gctx, `
			SELECT COUNT(*) FROM room_users ru
			INNER JOIN communities c ON c.room_id = ru.room_id`).Scan(&totalCommunityMembers)
	})

	// オンボーディング完了率（プロフィール + アバター + 初投稿）の元データ
	var onboardingComplete int
	g.Go(func() error {
		return r.DB.QueryRowContext(gctx, `
			SELECT COUNT(DISTINCT u.id) FROM users u
			INNER JOIN profiles p ON p.user_id = u.id AND p.avatar_media_id IS NOT NULL
			WHERE EXISTS (SELECT 1 FROM posts po WHERE po.user_id = u.id AND po.deleted_at IS NULL)`).Scan(&onboardingComplete)
	})

	// 初投稿までの平均時間（分）
	var avgSeconds sql.NullFloat64
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

	// セッションデータ（user_session_summaries テーブルが存在する場合のみ）。
	// テーブル未作成環境ではベストエフォートで無視するため、他クエリの失敗としては扱わない。
	g.Go(func() error {
		_ = r.loadSessionStats(gctx, s)
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 計算値
	if s.TotalPosts > 0 {
		s.AvgLikesPerPost = float64(s.TotalLikes) / float64(s.TotalPosts)
		s.AvgCommentsPerPost = float64(s.TotalComments) / float64(s.TotalPosts)
	}
	if s.TotalNotifications > 0 {
		s.NotificationReadRate = float64(s.ReadNotifications) / float64(s.TotalNotifications) * 100
	}
	if s.TotalUsers > 0 {
		s.DAUMAURatio = float64(s.DAU) / float64(s.TotalUsers) * 100
		if s.FrozenUsersCount > 0 {
			// 凍結を退会の近似値として使用
		}
	}
	if s.MAU > 0 {
		s.DAUMAURatio = float64(s.DAU) / float64(s.MAU) * 100
	}

	// コミュニティ平均メンバー数
	if s.TotalCommunities > 0 {
		s.AvgCommunityMembers = float64(totalCommunityMembers) / float64(s.TotalCommunities)
	}

	// ユーザーあたり平均コミュニティ参加数
	if s.TotalUsers > 0 {
		s.AvgCommunitiesPerUser = float64(totalCommunityMembers) / float64(s.TotalUsers)
	}

	// フォロー/フォロワー平均
	if s.TotalUsers > 0 {
		s.AvgFollowersPerUser = float64(s.TotalFollows) / float64(s.TotalUsers)
		s.AvgFollowingPerUser = float64(s.TotalFollows) / float64(s.TotalUsers)
	}

	// オンボーディング完了率（プロフィール + アバター + 初投稿）
	if s.TotalUsers > 0 {
		s.OnboardingCompleteRate = float64(onboardingComplete) / float64(s.TotalUsers) * 100
	}

	// 初投稿までの平均時間（分）
	if avgSeconds.Valid {
		s.AvgTimeToFirstPostMinutes = avgSeconds.Float64
	}

	// インフラ（in-memory）
	s.WebSocketConnections = metrics.Global.WSConnections()
	s.SSEConnections = metrics.Global.SSEConnections()
	s.ErrorRate5xx = metrics.Global.ErrorRate()
	s.P50ResponseTimeMs = metrics.Global.Percentile(50)
	s.P95ResponseTimeMs = metrics.Global.Percentile(95)
	s.P99ResponseTimeMs = metrics.Global.Percentile(99)

	return s, nil
}

func (r *MySQLAnalyticsRepository) loadSessionStats(ctx context.Context, s *model.AnalyticsSummary) error {
	// 平均セッション時間（秒）
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

	// 1日あたりの平均セッション数
	var avgSessions sql.NullFloat64
	if err := r.DB.QueryRowContext(ctx, `
		SELECT AVG(session_count) FROM user_session_summaries`).Scan(&avgSessions); err != nil {
		return err
	}
	if avgSessions.Valid {
		s.AvgSessionsPerDay = avgSessions.Float64
	}

	// 平均スクロール深度
	var avgScroll sql.NullFloat64
	if err := r.DB.QueryRowContext(ctx, `
		SELECT AVG(total_max_scroll_depth / view_count)
		FROM page_view_stats WHERE view_count > 0`).Scan(&avgScroll); err != nil {
		return err
	}
	if avgScroll.Valid {
		s.AvgScrollDepth = avgScroll.Float64
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

func (r *MySQLAnalyticsRepository) GetCommunityAnalytics(ctx context.Context, limit, offset int) ([]*model.CommunityStatItem, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM communities`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT c.id, c.name,
		       COUNT(DISTINCT ru.user_id) AS member_count,
		       COUNT(DISTINCT m.id) AS message_count
		FROM communities c
		LEFT JOIN room_users ru ON ru.room_id = c.room_id
		LEFT JOIN messages m ON m.room_id = c.room_id
		GROUP BY c.id, c.name
		ORDER BY member_count DESC
		LIMIT ? OFFSET ?`, limit, offset)
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

func (r *MySQLAnalyticsRepository) GetTimeSeries(ctx context.Context, granularity, from, to string) ([]*model.TimeSeriesPoint, error) {
	const dateFmt = "2006-01-02"

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
	if granularity == "hour" {
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

	between := fmt.Sprintf("created_at >= %s AND created_at < %s", sinceExpr, untilExpr)
	mqs := []metricQuery{
		{posts, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL AND %s GROUP BY lbl`, localTS, labelFmt, between)},
		{comments, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM posts WHERE parent_id IS NOT NULL AND deleted_at IS NULL AND %s GROUP BY lbl`, localTS, labelFmt, between)},
		{messages, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM messages WHERE %s GROUP BY lbl`, localTS, labelFmt, between)},
		{newUsers, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM users WHERE %s GROUP BY lbl`, localTS, labelFmt, between)},
		{likes, fmt.Sprintf(`SELECT DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as lbl, COUNT(*) FROM favorites WHERE %s GROUP BY lbl`, localTS, labelFmt, between)},
	}

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
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 範囲内の全スロットを生成してギャップを0で埋める
	var labels []string
	if granularity == "hour" {
		for t := fromT; t.Before(toT); t = t.Add(time.Hour) {
			labels = append(labels, t.Format("2006-01-02 15:00"))
		}
	} else {
		for t := fromT; t.Before(toT); t = t.AddDate(0, 0, 1) {
			labels = append(labels, t.Format("2006-01-02"))
		}
	}

	points := make([]*model.TimeSeriesPoint, len(labels))
	for i, l := range labels {
		points[i] = &model.TimeSeriesPoint{
			Label:    l,
			Posts:    posts[l],
			Comments: comments[l],
			Messages: messages[l],
			NewUsers: newUsers[l],
			Likes:    likes[l],
		}
	}
	return points, nil
}

