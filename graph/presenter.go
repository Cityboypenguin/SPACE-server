package graph

import (
	"fmt"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

const timeFormat = "2006-01-02T15:04:05Z07:00"

const deletedAccountDisplayName = "削除されたアカウント"

func toGraphUser(user *model.User) *gqlmodel.User {
	if user == nil {
		return nil
	}
	return &gqlmodel.User{
		ID:        encodeGraphID("user", user.ID),
		AccountID: user.AccountID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format(timeFormat),
		UpdatedAt: user.UpdatedAt.Format(timeFormat),
	}
}

func toGraphDeletedUser() *gqlmodel.User {
	return toGraphDeletedUserWithID(0)
}

func toGraphDeletedUserWithID(id int64) *gqlmodel.User {
	deletedAt := time.Unix(0, 0).Format(timeFormat)
	return &gqlmodel.User{
		ID:        encodeGraphID("user", id),
		AccountID: "deleted-account",
		Name:      deletedAccountDisplayName,
		Email:     "",
		Role:      "",
		Status:    "",
		CreatedAt: deletedAt,
		UpdatedAt: deletedAt,
	}
}

func toGraphAdministrator(admin *model.Administrator) *gqlmodel.Administrator {
	if admin == nil {
		return nil
	}
	return &gqlmodel.Administrator{
		ID:        encodeGraphID("administrator", admin.ID),
		Name:      admin.Name,
		Email:     admin.Email,
		CreatedAt: admin.CreatedAt.Format(timeFormat),
		UpdatedAt: admin.UpdatedAt.Format(timeFormat),
	}
}

func toGraphRoom(room *model.Room) *gqlmodel.Room {
	if room == nil {
		return nil
	}
	return &gqlmodel.Room{
		ID:        encodeGraphID("room", room.ID),
		Name:      room.Name,
		Type:      room.Type,
		CreatedAt: room.CreatedAt.Format(timeFormat),
		UpdatedAt: room.UpdatedAt.Format(timeFormat),
	}
}

func toGraphCommunity(c *model.Community, avatarURL string) *gqlmodel.Community {
	if c == nil {
		return nil
	}

	return &gqlmodel.Community{
		ID:          encodeGraphID("community", c.ID),
		RoomID:      encodeGraphID("room", c.RoomID),
		Name:        c.Name,
		Description: c.Description,
		AvatarURL:   avatarURL,
		CreatedAt:   c.CreatedAt.Format(timeFormat),
		UpdatedAt:   c.UpdatedAt.Format(timeFormat),
	}
}

func toGraphMessage(msg *model.Message) *gqlmodel.Message {
	if msg == nil {
		return nil
	}
	return &gqlmodel.Message{
		ID:        encodeGraphID("message", msg.ID),
		RoomID:    encodeGraphID("room", msg.RoomID),
		UserID:    encodeGraphID("user", msg.UserID),
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt.Format(timeFormat),
		UpdatedAt: msg.UpdatedAt.Format(timeFormat),
	}
}

func toGraphMedia(m *model.Media, url string) *gqlmodel.Media {
	if m == nil {
		return nil
	}
	return &gqlmodel.Media{
		ID:          encodeGraphID("media", m.ID),
		URL:         url,
		ContentType: m.ContentType,
		CreatedAt:   m.CreatedAt.Format(timeFormat),
	}
}

func toGraphPost(post *model.Post) *gqlmodel.Post {
	if post == nil {
		return nil
	}

	var gqlParent *gqlmodel.Post
	if post.ParentID != nil {
		gqlParent = &gqlmodel.Post{ID: encodeGraphID("post", *post.ParentID)}
	}

	var deletedAt *string
	if post.DeletedAt != nil {
		formatted := post.DeletedAt.Format(timeFormat)
		deletedAt = &formatted
	}

	return &gqlmodel.Post{
		ID:         encodeGraphID("post", post.ID),
		Content:    post.Content,
		CreatedAt:  post.CreatedAt.Format(timeFormat),
		UpdatedAt:  post.UpdatedAt.Format(timeFormat),
		DeletedAt:  deletedAt,
		User:       toGraphUser(&model.User{ID: post.UserID}),
		Parent:     gqlParent,
		ReplyCount: int32(post.ReplyCount),
	}
}

const notificationTargetTypePost = "post"

func toGraphNotification(n *model.Notification, actorMap map[int64]*model.User, postMap map[int64]*model.Post) *gqlmodel.Notification {
	if n == nil {
		return nil
	}
	gql := &gqlmodel.Notification{
		ID:        encodeGraphID("notification", n.ID),
		Type:      n.Type,
		Message:   n.Message,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt.Format(timeFormat),
	}
	if n.TargetType != nil {
		gql.TargetType = n.TargetType
	}
	if n.TargetID != nil {
		id := encodeGraphID(*n.TargetType, *n.TargetID)
		gql.TargetID = &id
		if n.TargetType != nil && *n.TargetType == notificationTargetTypePost {
			if p, ok := postMap[*n.TargetID]; ok {
				gql.TargetPost = toGraphPost(p)
			}
		}
	}
	if n.ActorID != nil {
		if u, ok := actorMap[*n.ActorID]; ok {
			gql.Actor = toGraphUser(u)
		} else {
			gql.Actor = toGraphDeletedUserWithID(*n.ActorID)
		}
	}
	return gql
}

func toGraphNotificationGroup(g *model.NotificationGroup, actorMap map[int64]*model.User, postMap map[int64]*model.Post) *gqlmodel.NotificationGroup {
	if g == nil {
		return nil
	}
	var key string
	if g.ActorID != nil {
		key = fmt.Sprintf("%s-%d", g.Type, *g.ActorID)
	} else {
		key = fmt.Sprintf("single-%d", g.LatestID)
	}
	gql := &gqlmodel.NotificationGroup{
		Key:         key,
		Type:        g.Type,
		Message:     g.Message,
		CreatedAt:   g.CreatedAt.Format(timeFormat),
		Count:       int32(g.Count),
		UnreadCount: int32(g.UnreadCount),
		LatestID:    encodeGraphID("notification", g.LatestID),
	}
	if g.TargetType != nil {
		gql.TargetType = g.TargetType
	}
	if g.TargetID != nil {
		id := encodeGraphID(*g.TargetType, *g.TargetID)
		gql.TargetID = &id
		if g.TargetType != nil && *g.TargetType == notificationTargetTypePost {
			if p, ok := postMap[*g.TargetID]; ok {
				gql.TargetPost = toGraphPost(p)
			}
		}
	}
	if g.ActorID != nil {
		if u, ok := actorMap[*g.ActorID]; ok {
			gql.Actor = toGraphUser(u)
		} else {
			gql.Actor = toGraphDeletedUserWithID(*g.ActorID)
		}
	}
	return gql
}

func toGraphInquiry(inq *model.Inquiry) *gqlmodel.Inquiry {
	return &gqlmodel.Inquiry{
		ID:        inq.ID,
		Name:      inq.Name,
		Email:     inq.Email,
		Category:  gqlmodel.InquiryCategory(inq.Category),
		Subject:   inq.Subject,
		Content:   inq.Content,
		Status:    gqlmodel.InquiryStatus(inq.Status),
		CreatedAt: inq.CreatedAt.Format(timeFormat),
		UpdatedAt: inq.UpdatedAt.Format(timeFormat),
	}
}

func toGraphTerms(t *model.TermsOfService, documentURL string) *gqlmodel.TermsOfService {
	if t == nil {
		return nil
	}
	return &gqlmodel.TermsOfService{
		ID:            encodeGraphID("terms", t.ID),
		Version:       t.Version,
		DocumentURL:   documentURL,
		EffectiveDate: t.EffectiveDate.Format(timeFormat),
		CreatedAt:     t.CreatedAt.Format(timeFormat),
	}
}

func toGraphProfile(user *model.User, profile *model.Profile, avatarURL *string) *gqlmodel.Profile {
	if user == nil {
		return nil
	}
	if profile == nil {
		return &gqlmodel.Profile{
			User:      toGraphUser(user),
			Username:  user.Name,
			CreatedAt: "0",
			UpdatedAt: "0",
		}
	}
	return &gqlmodel.Profile{
		User:      toGraphUser(user),
		Username:  user.Name,
		Bio:       &profile.Bio,
		AvatarURL: avatarURL,
		CreatedAt: profile.CreatedAt.Format(timeFormat),
		UpdatedAt: profile.UpdatedAt.Format(timeFormat),
	}
}

func toGraphAnnouncement(a *model.Announcement) *gqlmodel.Announcement {
	if a == nil {
		return nil
	}
	return &gqlmodel.Announcement{
		ID:        encodeGraphID("announcement", a.ID),
		Title:     a.Title,
		Body:      a.Body,
		CreatedAt: a.CreatedAt.Format(timeFormat),
		UpdatedAt: a.UpdatedAt.Format(timeFormat),
	}
}

func toGraphFavoriteUser(fu *model.FavoriteUser) *gqlmodel.FavoriteUser {
	if fu == nil {
		return nil
	}
	return &gqlmodel.FavoriteUser{
		ID:             encodeGraphID("favorite_user", fu.ID),
		UserID:         encodeGraphID("user", fu.UserID),
		FavoriteUserID: encodeGraphID("user", fu.FavoriteUserID),
		CreatedAt:      fu.CreatedAt.Format(timeFormat),
	}
}

func toGraphBlocker(b *model.Blocker) *gqlmodel.Blocker {
	if b == nil {
		return nil
	}
	return &gqlmodel.Blocker{
		ID:            encodeGraphID("blocker", b.ID),
		UserID:        encodeGraphID("user", b.UserID),
		BlockedUserID: encodeGraphID("user", b.BlockedUserID),
		CreatedAt:     b.CreatedAt.Format(timeFormat),
	}
}

func toGraphFavorite(f *model.Favorite) *gqlmodel.Favorite {
	if f == nil {
		return nil
	}
	return &gqlmodel.Favorite{
		ID:        encodeGraphID("favorite", f.ID),
		User:      toGraphUser(&model.User{ID: f.UserID}),
		Post:      toGraphPost(&model.Post{ID: f.PostID}),
		CreatedAt: f.CreatedAt.Format(timeFormat),
	}
}

func toGraphAnalyticsSummary(s *model.AnalyticsSummary) *gqlmodel.AnalyticsSummary {
	if s == nil {
		return nil
	}
	pageViewStats := make([]*gqlmodel.PageViewStat, 0, len(s.PageViewStats))
	for _, pv := range s.PageViewStats {
		pageViewStats = append(pageViewStats, &gqlmodel.PageViewStat{
			PagePath:           pv.PagePath,
			AvgDurationSeconds: pv.AvgDurationSeconds,
			AvgMaxScrollDepth:  pv.AvgMaxScrollDepth,
			TotalViews:         int32(pv.TotalViews),
		})
	}
	return &gqlmodel.AnalyticsSummary{
		TotalUsers:                  int32(s.TotalUsers),
		NewUsersToday:               int32(s.NewUsersToday),
		NewUsersThisWeek:            int32(s.NewUsersThisWeek),
		NewUsersThisMonth:           int32(s.NewUsersThisMonth),
		FrozenUsersCount:            int32(s.FrozenUsersCount),
		TotalPosts:                  int32(s.TotalPosts),
		TotalComments:               int32(s.TotalComments),
		TotalDeletedPosts:           int32(s.TotalDeletedPosts),
		TotalLikes:                  int32(s.TotalLikes),
		TotalCommunities:            int32(s.TotalCommunities),
		TotalMessages:               int32(s.TotalMessages),
		TotalReports:                int32(s.TotalReports),
		TotalBlocks:                 int32(s.TotalBlocks),
		TotalInquiries:              int32(s.TotalInquiries),
		Dau:                         int32(s.DAU),
		Wau:                         int32(s.WAU),
		Mau:                         int32(s.MAU),
		DauMauRatio:                 s.DAUMAURatio,
		PostsToday:                  int32(s.PostsToday),
		CommentsToday:               int32(s.CommentsToday),
		MessagesToday:               int32(s.MessagesToday),
		AvgLikesPerPost:             s.AvgLikesPerPost,
		AvgCommentsPerPost:          s.AvgCommentsPerPost,
		PostsTextOnly:               int32(s.PostsTextOnly),
		PostsWithImage:              int32(s.PostsWithImage),
		PostsWithVideo:              int32(s.PostsWithVideo),
		UniqueDMSenders:             int32(s.UniqueDMSenders),
		ActiveCommunitiesLast30Days: int32(s.ActiveCommunitiesLast30Days),
		AvgCommunityMembers:         s.AvgCommunityMembers,
		AvgCommunitiesPerUser:       s.AvgCommunitiesPerUser,
		TotalFollows:                int32(s.TotalFollows),
		AvgFollowersPerUser:         s.AvgFollowersPerUser,
		AvgFollowingPerUser:         s.AvgFollowingPerUser,
		UsersWithProfile:            int32(s.UsersWithProfile),
		UsersWithAvatar:             int32(s.UsersWithAvatar),
		UsersWithPost:               int32(s.UsersWithPost),
		OnboardingCompleteRate:      s.OnboardingCompleteRate,
		AvgTimeToFirstPostMinutes:   s.AvgTimeToFirstPostMinutes,
		TotalNotifications:          int32(s.TotalNotifications),
		ReadNotifications:           int32(s.ReadNotifications),
		NotificationReadRate:        s.NotificationReadRate,
		PendingReports:              int32(s.PendingReports),
		ResolvedReports:             int32(s.ResolvedReports),
		WebSocketConnections:        int32(s.WebSocketConnections),
		SseConnections:              int32(s.SSEConnections),
		ErrorRate5xx:                s.ErrorRate5xx,
		P50ResponseTimeMs:           s.P50ResponseTimeMs,
		P95ResponseTimeMs:           s.P95ResponseTimeMs,
		P99ResponseTimeMs:           s.P99ResponseTimeMs,
		AvgSessionDurationSeconds:   s.AvgSessionDurationSeconds,
		AvgSessionsPerDay:           s.AvgSessionsPerDay,
		AvgScrollDepth:              s.AvgScrollDepth,
		PageViewStats:               pageViewStats,
	}
}

func toGraphCommunityStatItem(c *model.CommunityStatItem) *gqlmodel.CommunityStatItem {
	return &gqlmodel.CommunityStatItem{
		CommunityID:  encodeGraphID("community", c.CommunityID),
		Name:         c.Name,
		MemberCount:  int32(c.MemberCount),
		MessageCount: int32(c.MessageCount),
	}
}
