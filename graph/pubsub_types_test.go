package graph

import (
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
)

// TestPubSubCodec_RoundTripsEverySubscriptionPayload は、subscription に流れる型が
// すべて台をまたげることを確かめる。
//
// 登録し忘れると、その subscription だけが「他の台で起きたことは届かない」状態になる。
// 1台で動かしている間は何も起きないので、テストでしか捕まえられない。
//
// subscription を足したら、その型をここに足すこと。
func TestPubSubCodec_RoundTripsEverySubscriptionPayload(t *testing.T) {
	c := NewPubSubCodec()
	startedAt := time.Unix(1700000000, 0).UTC()

	cases := []struct {
		name  string
		value any
		check func(t *testing.T, got any)
	}{
		{
			name:  "message",
			value: &gqlmodel.Message{ID: "m1", RoomID: "r1", UserID: "u1", Content: "やあ"},
			check: func(t *testing.T, got any) {
				m, ok := got.(*gqlmodel.Message)
				if !ok {
					t.Fatalf("型が %T", got)
				}
				// RoomID と UserID は受け取った台が匿名表示や権限を決めるのに使う。
				// ここが落ちると、授業内チャットで実名が出るなどの形で効く。
				if m.ID != "m1" || m.RoomID != "r1" || m.UserID != "u1" || m.Content != "やあ" {
					t.Fatalf("中身が変わっている: %+v", m)
				}
			},
		},
		{
			name:  "question",
			value: &gqlmodel.Question{ID: "q1", RoomID: "r1", Body: "これは"},
			check: func(t *testing.T, got any) {
				q, ok := got.(*gqlmodel.Question)
				if !ok || q.ID != "q1" || q.RoomID != "r1" || q.Body != "これは" {
					t.Fatalf("中身が変わっている: %#v", got)
				}
			},
		},
		{
			name:  "answer",
			value: &gqlmodel.Answer{ID: "a1", QuestionID: "q1", Body: "こうです"},
			check: func(t *testing.T, got any) {
				a, ok := got.(*gqlmodel.Answer)
				if !ok || a.ID != "a1" || a.QuestionID != "q1" || a.Body != "こうです" {
					t.Fatalf("中身が変わっている: %#v", got)
				}
			},
		},
		{
			name: "poll",
			value: &gqlmodel.Poll{ID: "p1", RoomID: "r1", Question: "どれ",
				Options: []*gqlmodel.PollOption{{ID: "o1", Label: "A", VoteCount: 2}}},
			check: func(t *testing.T, got any) {
				p, ok := got.(*gqlmodel.Poll)
				if !ok || p.ID != "p1" || len(p.Options) != 1 || p.Options[0].Label != "A" {
					t.Fatalf("中身が変わっている: %#v", got)
				}
			},
		},
		{
			name:  "room_read_status",
			value: &gqlmodel.RoomReadStatusUpdate{UserID: "u1", LastReadAt: "2026-01-01 00:00:00"},
			check: func(t *testing.T, got any) {
				u, ok := got.(*gqlmodel.RoomReadStatusUpdate)
				if !ok || u.UserID != "u1" {
					t.Fatalf("中身が変わっている: %#v", got)
				}
			},
		},
		{
			// 取り込み状況だけは値で流している。ポインタで登録すると
			// 受け手の型アサーションが通らず、管理画面の進捗が止まる。
			name:  "course_import_status",
			value: courseimport.Status{State: courseimport.StateRunning, Year: 2026, Processed: 3, Total: 10, StartedAt: &startedAt},
			check: func(t *testing.T, got any) {
				s, ok := got.(courseimport.Status)
				if !ok {
					t.Fatalf("型が %T（値で戻ること）", got)
				}
				if s.State != courseimport.StateRunning || s.Year != 2026 || s.Processed != 3 || s.Total != 10 {
					t.Fatalf("中身が変わっている: %+v", s)
				}
				if s.StartedAt == nil || !s.StartedAt.Equal(startedAt) {
					t.Fatalf("開始時刻が落ちている: %+v", s.StartedAt)
				}
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := c.Encode(tt.value)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := c.Decode(raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			tt.check(t, got)
		})
	}
}
