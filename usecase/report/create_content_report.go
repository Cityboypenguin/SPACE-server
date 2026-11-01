package report

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// RoomReadAccess はルームを閲覧してよいかの判定（実体は chat.AccessPolicy）。
type RoomReadAccess interface {
	EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error)
}

// QuestionReader は通報する質問を引く口。無ければ nil を返す。
type QuestionReader interface {
	GetQuestionByID(ctx context.Context, id int64) (*model.Question, error)
}

// AnswerReader は通報する回答を引く口。無ければ nil を返す。
type AnswerReader interface {
	GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error)
}

// PollReader は通報する投票と選択肢を引く口。投票が無ければ nil を返す。
type PollReader interface {
	GetPollByID(ctx context.Context, id int64) (*model.Poll, error)
	ListOptionsWithResults(ctx context.Context, pollID, viewerUserID int64) ([]*repository.PollOptionResult, error)
}

// CreateContentReportDeps は通報時点の内容を写すのに要るものだけ。
type CreateContentReportDeps struct {
	Reports *CreateReportUsecase
	// Access を飛ばすと、ID を指定するだけで他人の DM の内容を通報に写し取れてしまう。
	Access    RoomReadAccess
	Messages  repository.MessageReader
	Questions QuestionReader
	Answers   AnswerReader
	Polls     PollReader
	Media     repository.MediaReader
	Users     userusecase.GetUserByIDUseCase
}

// CreateContentReportUsecase はメッセージ・質問・回答・投票の通報を受け付け、
// その時点の内容を写して残す。
type CreateContentReportUsecase struct {
	deps CreateContentReportDeps
}

func NewCreateContentReportUsecase(deps CreateContentReportDeps) *CreateContentReportUsecase {
	return &CreateContentReportUsecase{deps: deps}
}

type CreateContentReportInput struct {
	ReporterID   int64
	TargetType   model.ReportTargetType
	TargetID     int64
	Reason       string
	CustomReason *string
}

// reportTarget は通報対象の種類ごとの違いを吸収した、写すのに要る情報。
type reportTarget struct {
	roomID     int64
	authorID   int64
	content    string
	parentBody *string
	createdAt  time.Time
	updatedAt  time.Time
	// loadExtras は閲覧権限を確かめたあとに、添付と選択肢を snapshot へ足す。
	loadExtras func(ctx context.Context, snapshot *model.ReportedContent) error
}

func (u *CreateContentReportUsecase) Execute(ctx context.Context, input CreateContentReportInput) (*model.Report, error) {
	target, err := u.loadTarget(ctx, input)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, apperr.New(apperr.CodeNotFound, "通報の対象が見つかりません")
	}

	room, err := u.deps.Access.EnsureReadAccess(ctx, target.roomID)
	if err != nil {
		return nil, err
	}

	if target.authorID == input.ReporterID {
		return nil, apperr.New(apperr.CodeInvalidInput, "自分の投稿を通報することはできません")
	}

	snapshot, err := u.buildSnapshot(ctx, input.TargetID, target, room)
	if err != nil {
		return nil, err
	}

	return u.deps.Reports.Execute(ctx, CreateReportInput{
		ReporterID:      input.ReporterID,
		TargetType:      input.TargetType,
		TargetID:        strconv.FormatInt(input.TargetID, 10),
		Reason:          input.Reason,
		CustomReason:    input.CustomReason,
		ReportedContent: snapshot,
	})
}

// loadTarget は対象を引く。無ければ nil を返す。
func (u *CreateContentReportUsecase) loadTarget(ctx context.Context, input CreateContentReportInput) (*reportTarget, error) {
	switch input.TargetType {
	case model.TargetMessage:
		msg, err := u.deps.Messages.GetMessageByID(ctx, input.TargetID)
		if err != nil || msg == nil {
			return nil, err
		}
		return &reportTarget{
			roomID:     msg.RoomID,
			authorID:   msg.UserID,
			content:    msg.Content,
			createdAt:  msg.CreatedAt,
			updatedAt:  msg.UpdatedAt,
			loadExtras: u.attachMedia(input.TargetID, u.deps.Media.ListByMessageIDs),
		}, nil
	case model.TargetQuestion:
		question, err := u.deps.Questions.GetQuestionByID(ctx, input.TargetID)
		if err != nil || question == nil {
			return nil, err
		}
		return &reportTarget{
			roomID:     question.RoomID,
			authorID:   question.AskerUserID,
			content:    question.Body,
			createdAt:  question.CreatedAt,
			updatedAt:  question.UpdatedAt,
			loadExtras: u.attachMedia(input.TargetID, u.deps.Media.ListByQuestionIDs),
		}, nil
	case model.TargetAnswer:
		answer, err := u.deps.Answers.GetAnswerByID(ctx, input.TargetID)
		if err != nil || answer == nil {
			return nil, err
		}
		// 回答はルームを直接持たないので、質問からたどる。
		question, err := u.deps.Questions.GetQuestionByID(ctx, answer.QuestionID)
		if err != nil || question == nil {
			return nil, err
		}
		return &reportTarget{
			roomID:     question.RoomID,
			authorID:   answer.AuthorUserID,
			content:    answer.Body,
			parentBody: &question.Body,
			createdAt:  answer.CreatedAt,
			updatedAt:  answer.UpdatedAt,
			loadExtras: u.attachMedia(input.TargetID, u.deps.Media.ListByAnswerIDs),
		}, nil
	case model.TargetPoll:
		poll, err := u.deps.Polls.GetPollByID(ctx, input.TargetID)
		if err != nil || poll == nil {
			return nil, err
		}
		return &reportTarget{
			roomID:    poll.RoomID,
			authorID:  poll.AuthorUserID,
			content:   poll.Question,
			createdAt: poll.CreatedAt,
			updatedAt: poll.UpdatedAt,
			loadExtras: func(ctx context.Context, snapshot *model.ReportedContent) error {
				options, err := u.deps.Polls.ListOptionsWithResults(ctx, input.TargetID, input.ReporterID)
				if err != nil {
					return err
				}
				for _, option := range options {
					snapshot.PollOptions = append(snapshot.PollOptions, option.Option.Label)
				}
				return nil
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported report target: %s", input.TargetType)
	}
}

func (u *CreateContentReportUsecase) attachMedia(
	id int64,
	list func(context.Context, []int64) (map[int64][]*model.Media, error),
) func(context.Context, *model.ReportedContent) error {
	return func(ctx context.Context, snapshot *model.ReportedContent) error {
		mediaByParent, err := list(ctx, []int64{id})
		if err != nil {
			return err
		}
		for _, m := range mediaByParent[id] {
			snapshot.Media = append(snapshot.Media, model.ReportedContentMedia{
				ID:          m.ID,
				StorageKey:  m.StorageKey,
				ContentType: m.ContentType,
				Width:       m.Width,
				Height:      m.Height,
				CreatedAt:   m.CreatedAt,
			})
		}
		return nil
	}
}

// buildSnapshot は通報された1件だけを写す。メッセージの前後の発言や返信先は写さない
// （DM は管理者でも読めない決まりなので、通報された発言以外を持ち出さないため）。
func (u *CreateContentReportUsecase) buildSnapshot(ctx context.Context, id int64, target *reportTarget, room *model.Room) (*model.ReportedContent, error) {
	authorID := target.authorID
	snapshot := &model.ReportedContent{
		ContentID:  id,
		RoomID:     room.ID,
		RoomType:   room.Type,
		RoomName:   room.Name,
		AuthorID:   &authorID,
		Content:    target.content,
		Media:      []model.ReportedContentMedia{},
		ParentBody: target.parentBody,
		CreatedAt:  target.createdAt,
		UpdatedAt:  target.updatedAt,
	}
	if err := target.loadExtras(ctx, snapshot); err != nil {
		return nil, err
	}

	author, err := u.deps.Users.Execute(ctx, target.authorID)
	if err != nil {
		return nil, err
	}
	if author != nil {
		snapshot.AuthorName = author.Name
		snapshot.AuthorAccountID = author.AccountID
	}
	return snapshot, nil
}
