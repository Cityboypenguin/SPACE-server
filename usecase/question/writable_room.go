package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

// requireWritableQuestionRoom rejects the operation unless the course room that
// questionID belongs to is currently writable for the caller.
func requireWritableQuestionRoom(ctx context.Context, questionRepo repository.QuestionRepository, requireWritable course.RequireWritableCourseRoomUseCase, questionID int64) error {
	q, err := questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q == nil {
		return apperr.NotFound("質問が見つかりません")
	}
	_, err = requireWritable.Execute(ctx, q.RoomID)
	return err
}
