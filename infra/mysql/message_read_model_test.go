package mysql

import (
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// hasMoreBefore / hasMoreAfter の判定は「どの条件を DB に聞くか」を決める純関数と
// 「聞く」部分に分けてある。ここでは前者（＝境界条件の組み立て）だけを DB 無しで確かめる。

func ptrInt64(v int64) *int64 { return &v }

func msgs(ids ...int64) []*model.Message {
	out := make([]*model.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, &model.Message{ID: id})
	}
	return out
}

func TestResolveCursor_Priority(t *testing.T) {
	at := time.Unix(1700000000, 0)
	c := resolveCursor(repository.MessageCursor{
		BeforeID:  ptrInt64(10),
		AfterID:   ptrInt64(20),
		AfterTime: &at,
	})
	if c.AfterTime == nil || c.AfterID != nil || c.BeforeID != nil {
		t.Fatalf("resolveCursor = %+v, want AfterTime only", c)
	}

	c = resolveCursor(repository.MessageCursor{BeforeID: ptrInt64(10), AfterID: ptrInt64(20)})
	if c.AfterID == nil || c.BeforeID != nil {
		t.Fatalf("resolveCursor = %+v, want AfterID only", c)
	}
}

func TestBoundaryProbes(t *testing.T) {
	afterTime := time.Unix(1700000000, 0)

	tests := []struct {
		name  string
		items []*model.Message
		curso repository.MessageCursor
		older *boundaryProbe
		newer *boundaryProbe
	}{
		{
			name:  "最新ページ（カーソル無し・件数あり）",
			items: msgs(5, 6, 7),
			older: &boundaryProbe{column: "id", op: "<", value: 5},
			newer: &boundaryProbe{column: "id", op: ">", value: 7},
		},
		{
			name:  "カーソル無しで0件＝部屋が空",
			items: nil,
			older: nil,
			newer: nil,
		},
		{
			name:  "before ページング（件数あり）",
			items: msgs(3, 4),
			curso: repository.MessageCursor{BeforeID: ptrInt64(5)},
			older: &boundaryProbe{column: "id", op: "<", value: 3},
			newer: &boundaryProbe{column: "id", op: ">", value: 4},
		},
		{
			name:  "before ページングで0件＝先頭。古い側は無いと確定し、新しい側はカーソル位置から調べる",
			items: nil,
			curso: repository.MessageCursor{BeforeID: ptrInt64(5)},
			older: nil,
			newer: &boundaryProbe{column: "id", op: ">=", value: 5},
		},
		{
			name:  "after ページングで0件＝末尾。新しい側は無いと確定し、古い側はカーソル位置から調べる",
			items: nil,
			curso: repository.MessageCursor{AfterID: ptrInt64(9)},
			older: &boundaryProbe{column: "id", op: "<=", value: 9},
			newer: nil,
		},
		{
			name:  "afterTime で0件＝その時刻以降は無い。古い側は時刻で調べる",
			items: nil,
			curso: repository.MessageCursor{AfterTime: &afterTime},
			older: &boundaryProbe{column: "created_at", op: "<=", value: afterTime.Unix()},
			newer: nil,
		},
		{
			name:  "afterTime で件数あり（以前は hasMoreBefore を true 固定していた箇所）",
			items: msgs(1, 2),
			curso: repository.MessageCursor{AfterTime: &afterTime},
			older: &boundaryProbe{column: "id", op: "<", value: 1},
			newer: &boundaryProbe{column: "id", op: ">", value: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := olderBoundary(tt.items, tt.curso); !probeEqual(got, tt.older) {
				t.Errorf("olderBoundary = %v, want %v", format(got), format(tt.older))
			}
			if got := newerBoundary(tt.items, tt.curso); !probeEqual(got, tt.newer) {
				t.Errorf("newerBoundary = %v, want %v", format(got), format(tt.newer))
			}
		})
	}
}

func probeEqual(a, b *boundaryProbe) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func format(p *boundaryProbe) string {
	if p == nil {
		return "nil"
	}
	return p.column + " " + p.op + " " + time.Unix(p.value, 0).UTC().Format(time.RFC3339)
}

func TestBuildMessagePageQuery_OrderAndArgs(t *testing.T) {
	// 過去方向は DESC で引いて呼び出し側で反転する（ascOrder=false）。
	_, args, ascOrder := buildMessagePageQuery(3, 10, repository.MessageCursor{BeforeID: ptrInt64(50)})
	if ascOrder {
		t.Error("before ページングは DESC で引くはず")
	}
	// limit+1 件引いて「もう1件あるか」ではなく、実測に使うための余裕を持たせている。
	if len(args) != 3 || args[2] != 11 {
		t.Errorf("args = %v, want [roomID beforeID limit+1]", args)
	}

	_, _, ascOrder = buildMessagePageQuery(3, 10, repository.MessageCursor{AfterID: ptrInt64(50)})
	if !ascOrder {
		t.Error("after ページングは ASC で引くはず")
	}
}
