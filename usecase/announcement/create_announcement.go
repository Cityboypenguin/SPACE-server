package announcement

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/async"
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

	// async は全ユーザーぶんの通知をリクエストの外で流すための実行口。
	// チャット配信と同じ internal/async.Runner（投げっぱなしの流儀はサーバで1つ）。
	// nil なら同期実行する（配線しない起動経路とテスト向け）。
	async *async.Runner
}

func NewCreateAnnouncementUseCase(
	announcementRepo repository.AnnouncementRepository,
	notificationPublisher notificationuc.NotificationPublisher,
	asyncRunner *async.Runner,
) *CreateAnnouncementUseCase {
	return &CreateAnnouncementUseCase{
		announcementRepo:      announcementRepo,
		notificationPublisher: notificationPublisher,
		async:                 asyncRunner,
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

	// 全ユーザーへの通知はリクエストの外で送る（保存は DB 内の INSERT ... SELECT。
	// お知らせの登録をここで待たせる理由が無い）。
	//
	// 以前は素の go func() + context.Background() だった。(1) 停止時に誰も待たないので
	// DB を閉じたあとに走って全滅しうる、(2) panic がプロセスを落とす、(3) ctx の値
	// （認証情報・リクエストID）が落ちてログが追えない、の3つがチャット配信側と
	// 揃っていなかった。Runner へ渡せば3つともチャットと同じ扱いになる。
	u.publishToAllUsers(ctx, a, title)

	return a, nil
}

// publishToAllUsers は全ユーザーへのお知らせ通知を Runner 経由で流す。
// Runner が配線されていなければその場で実行する（挙動は同じで、待つかどうかだけが違う）。
//
// 中身が PublishToAllActiveUsers 1本なのは、宛先の列挙をここでやめたため。
// 以前はこの関数が (a) 全アクティブユーザーIDの取得、(b) 人数ぶんの PublishParams の
// 組み立て、(c) 一括保存、(d) 全員への配信、を順に並べていた。(a)(b) は
// 「行を作るために中身をアプリへ運ぶ」だけの処理で、利用者数に比例してメモリを食う。
// 保存を DB 内へ、配信先を接続中の利用者へ寄せたので、ここに残るのは
// 「どの通知を、どのお知らせについて配るか」の指定だけになる。
func (u *CreateAnnouncementUseCase) publishToAllUsers(ctx context.Context, a *model.Announcement, title string) {
	notify := func(ctx context.Context) {
		if err := u.notificationPublisher.PublishToAllActiveUsers(ctx, notificationuc.BroadcastParams{
			Type:       notificationuc.TypeAnnouncement,
			TargetType: notificationuc.TargetAnnouncement,
			TargetID:   a.ID,
			Message:    title,
		}); err != nil {
			logger.Log.Error().Err(err).
				Str("component", "announcement").
				Int64("announcement_id", a.ID).
				Msg("failed to publish announcement notifications")
		}
	}

	if u.async == nil {
		notify(context.WithoutCancel(ctx))
		return
	}
	u.async.Go(ctx, "announcement_created", notify)
}
