package notification

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// IssueStreamTicketUseCase は /events（通知の SSE）へ繋ぐための使い捨てチケットを発行する。
//
// 認証済みのユーザーが、接続の直前に1枚もらって URL に載せる。JWT を URL に載せない
// ための仕組み（理由は repository.SSETicketRepository のコメント参照）。
type IssueStreamTicketUseCase interface {
	Execute(ctx context.Context, userID int64) (string, error)
}

var _ IssueStreamTicketUseCase = &issueStreamTicketInteractor{}

type issueStreamTicketInteractor struct {
	repo repository.SSETicketRepository
}

func NewIssueStreamTicketUseCase(repo repository.SSETicketRepository) IssueStreamTicketUseCase {
	return &issueStreamTicketInteractor{repo: repo}
}

// streamTicketBytes はチケットの乱数長。128bit あれば、30秒の寿命のあいだに
// 総当たりで有効なチケットを引き当てることは現実的に不可能。
const streamTicketBytes = 16

func (uc *issueStreamTicketInteractor) Execute(ctx context.Context, userID int64) (string, error) {
	buf := make([]byte, streamTicketBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate stream ticket: %w", err)
	}
	// URL に載せるので RawURLEncoding（+ / = が出ない＝エスケープ不要）。
	ticket := base64.RawURLEncoding.EncodeToString(buf)

	if err := uc.repo.Issue(ctx, ticket, userID); err != nil {
		return "", fmt.Errorf("failed to store stream ticket: %w", err)
	}
	return ticket, nil
}
