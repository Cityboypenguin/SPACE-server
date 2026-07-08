package model

type AnalyticsSummary struct {
	// 基本集計
	TotalUsers        int
	NewUsersToday     int
	NewUsersThisWeek  int
	NewUsersThisMonth int
	FrozenUsersCount  int
	TotalPosts        int
	TotalComments     int
	TotalLikes        int
	TotalCommunities  int
	TotalMessages     int
	TotalReports      int
	TotalBlocks       int
	TotalInquiries    int
	TotalDeletedPosts int

	// アクティビティ
	CurrentActiveUsers int
	DAU                int
	WAU                int
	MAU                int
	DAUMAURatio        float64

	// コンテンツ（日次）
	PostsToday    int
	CommentsToday int
	MessagesToday int

	// エンゲージメント
	AvgLikesPerPost   float64
	AvgCommentsPerPost float64

	// 投稿種別
	PostsTextOnly  int
	PostsWithImage int
	PostsWithVideo int

	// DM
	UniqueDMSenders int

	// コミュニティ
	ActiveCommunitiesLast30Days int
	AvgCommunityMembers         float64
	AvgCommunitiesPerUser       float64

	// ソーシャル
	TotalFollows       int
	AvgFollowersPerUser float64
	AvgFollowingPerUser float64

	// オンボーディング
	UsersWithProfile     int
	UsersWithAvatar      int
	UsersWithPost        int
	OnboardingCompleteRate float64
	AvgTimeToFirstPostMinutes float64

	// 通知
	TotalNotifications   int
	ReadNotifications    int
	NotificationReadRate float64

	// 通報ステータス
	PendingReports  int
	ResolvedReports int

	// インフラ（in-memory）
	WebSocketConnections int
	SSEConnections       int
	ErrorRate5xx         float64
	P50ResponseTimeMs    float64
	P95ResponseTimeMs    float64
	P99ResponseTimeMs    float64

	// セッション（クライアント送信データ）
	AvgSessionDurationSeconds float64
	AvgSessionsPerDay         float64
	AvgScrollDepth            float64
	PageViewStats             []*PageViewStat
}

type PageViewStat struct {
	PagePath            string
	AvgDurationSeconds  float64
	AvgMaxScrollDepth   float64
	TotalViews          int
}

type CommunityStatItem struct {
	CommunityID  int64
	Name         string
	MemberCount  int
	MessageCount int
}

type TimeSeriesPoint struct {
	Label       string
	Posts       int
	Comments    int
	Messages    int
	NewUsers    int
	Likes       int
	ActiveUsers int
}
