package graph

import (
	"context"
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

func TestPollResultsHidden(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour).Format(timeFormat)
	past := now.Add(-time.Hour).Format(timeFormat)
	poll := func(deadline *string) *gqlmodel.Poll {
		return &gqlmodel.Poll{Deadline: deadline, User: &gqlmodel.User{ID: encodeGraphID("user", 10)}}
	}
	as := func(id int64, role string) context.Context {
		return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: role})
	}
	notVoted := []*repository.PollOptionResult{{VoteCount: 3}, {VoteCount: 1}}
	voted := []*repository.PollOptionResult{{VoteCount: 3, VotedByMe: true}, {VoteCount: 1}}

	cases := []struct {
		name    string
		ctx     context.Context
		poll    *gqlmodel.Poll
		results []*repository.PollOptionResult
		want    bool
	}{
		{"期限前・未投票の人には伏せる", as(20, "user"), poll(&future), notVoted, true},
		{"期限が無ければ見せる", as(20, "user"), poll(nil), notVoted, false},
		{"期限が過ぎたら見せる", as(20, "user"), poll(&past), notVoted, false},
		{"投票済みなら期限前でも見せる", as(20, "user"), poll(&future), voted, false},
		{"作成者には見せる", as(10, "user"), poll(&future), notVoted, false},
		{"管理者には見せる", as(1, "administrator"), poll(&future), notVoted, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pollResultsHidden(tc.ctx, tc.poll, tc.results, now); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
