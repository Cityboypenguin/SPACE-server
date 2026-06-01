package announcement

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type CreateAnnouncementInput struct {
	Title   string
	Body    string
	AdminID int64
}

type CreateAnnouncementUseCase struct {
	announcementRepo      repository.AnnouncementRepository
	notificationPublisher notificationuc.NotificationPublisher
}

func NewCreateAnnouncementUseCase(
	announcementRepo repository.AnnouncementRepository,
	notificationPublisher notificationuc.NotificationPublisher,
) *CreateAnnouncementUseCase {
	return &CreateAnnouncementUseCase{
		announcementRepo:      announcementRepo,
		notificationPublisher: notificationPublisher,
	}
}

func (u *CreateAnnouncementUseCase) Execute(ctx context.Context, input CreateAnnouncementInput) (*model.Announcement, error) {
	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)

	if title == "" || body == "" {
		return nil, errors.New("title and body are required")
	}
	if len(title) > 255 {
		return nil, errors.New("title must be 255 characters or less")
	}

	a := &model.Announcement{
		Title:     title,
		Body:      body,
		AdminID:   input.AdminID,
		CreatedAt: time.Now(),
	}

	if err := u.announcementRepo.Save(ctx, a); err != nil {
		return nil, err
	}

	// 全ユーザーへの通知を非同期で送信 (バッチINSERT)
	go func() {
		bgCtx := context.Background()
		userIDs, err := u.announcementRepo.ListAllUserIDs(bgCtx)
		if err != nil {
			logger.Log.Error().Err(err).Msg("announcement: failed to list user ids")
			return
		}
		targetType := notificationuc.TargetAnnouncement
		params := make([]notificationuc.PublishParams, 0, len(userIDs))
		for _, userID := range userIDs {
			params = append(params, notificationuc.PublishParams{
				UserID:     userID,
				Type:       notificationuc.TypeAnnouncement,
				Message:    title,
				TargetType: &targetType,
				TargetID:   &a.ID,
			})
		}
		if err := u.notificationPublisher.PublishBatch(bgCtx, params); err != nil {
			logger.Log.Error().Err(err).Msg("announcement: failed to publish notifications")
		}
	}()

	return a, nil
}
