package report

import (
	"context"
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// RoomReadAccess はルームを閲覧してよいかの判定（実体は chat.AccessPolicy）。
type RoomReadAccess interface {
	EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error)
}

// CreateMessageReportDeps はメッセージの通報に要るものだけ。
type CreateMessageReportDeps struct {
	Reports *CreateReportUsecase
	// Access を飛ばすと、ID を指定するだけで他人の DM の内容を通報に写し取れてしまう。
	Access   RoomReadAccess
	Messages repository.MessageReader
	Media    repository.MediaReader
	Users    userusecase.GetUserByIDUseCase
}

// CreateMessageReportUsecase はメッセージの通報を受け付け、その時点の内容を写して残す。
type CreateMessageReportUsecase struct {
	deps CreateMessageReportDeps
}

func NewCreateMessageReportUsecase(deps CreateMessageReportDeps) *CreateMessageReportUsecase {
	return &CreateMessageReportUsecase{deps: deps}
}

type CreateMessageReportInput struct {
	ReporterID   int64
	MessageID    int64
	Reason       string
	CustomReason *string
}

func (u *CreateMessageReportUsecase) Execute(ctx context.Context, input CreateMessageReportInput) (*model.Report, error) {
	msg, err := u.deps.Messages.GetMessageByID(ctx, input.MessageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, apperr.New(apperr.CodeNotFound, "メッセージが見つかりません")
	}

	room, err := u.deps.Access.EnsureReadAccess(ctx, msg.RoomID)
	if err != nil {
		return nil, err
	}

	if msg.UserID == input.ReporterID {
		return nil, apperr.New(apperr.CodeInvalidInput, "自分のメッセージを通報することはできません")
	}

	snapshot, err := u.buildSnapshot(ctx, msg, room)
	if err != nil {
		return nil, err
	}

	return u.deps.Reports.Execute(ctx, CreateReportInput{
		ReporterID:      input.ReporterID,
		TargetType:      model.TargetMessage,
		TargetID:        strconv.FormatInt(msg.ID, 10),
		Reason:          input.Reason,
		CustomReason:    input.CustomReason,
		ReportedMessage: snapshot,
	})
}

// buildSnapshot は通報された1件だけを写す。前後の発言や返信先は写さない
// （DM は管理者でも読めない決まりなので、通報された発言以外を持ち出さないため）。
func (u *CreateMessageReportUsecase) buildSnapshot(ctx context.Context, msg *model.Message, room *model.Room) (*model.ReportedMessage, error) {
	mediaByMessage, err := u.deps.Media.ListByMessageIDs(ctx, []int64{msg.ID})
	if err != nil {
		return nil, err
	}
	media := make([]model.ReportedMessageMedia, 0, len(mediaByMessage[msg.ID]))
	for _, m := range mediaByMessage[msg.ID] {
		media = append(media, model.ReportedMessageMedia{
			ID:          m.ID,
			StorageKey:  m.StorageKey,
			ContentType: m.ContentType,
			Width:       m.Width,
			Height:      m.Height,
			CreatedAt:   m.CreatedAt,
		})
	}

	snapshot := &model.ReportedMessage{
		MessageID: msg.ID,
		RoomID:    room.ID,
		RoomType:  room.Type,
		RoomName:  room.Name,
		Content:   msg.Content,
		Media:     media,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	authorID := msg.UserID
	snapshot.AuthorID = &authorID
	author, err := u.deps.Users.Execute(ctx, msg.UserID)
	if err != nil {
		return nil, err
	}
	if author != nil {
		snapshot.AuthorName = author.Name
		snapshot.AuthorAccountID = author.AccountID
	}
	return snapshot, nil
}
