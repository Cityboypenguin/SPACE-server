package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type reportRooms map[int64]*model.Room

func (r reportRooms) Execute(_ context.Context, id int64) (*model.Room, error) {
	if room := r[id]; room != nil {
		return room, nil
	}
	return nil, fmt.Errorf("room not found: %d", id)
}

// スレッドと親を分けて写す前の通報は、スレッドの題名が場所の名前に入っている。
// 表示のときに親のコミュニティへ読み替え、題名はスレッドの題名として出す。
func TestWithThreadParent_ReadsOldReportsAsTheParent(t *testing.T) {
	parentID := int64(4)
	r := &Resolver{}
	r.GetRoomUseCase = reportRooms{
		4: {ID: 4, Type: model.RoomTypeCommunity, Name: "写真部"},
		5: {ID: 5, Type: model.RoomTypeCommunity, Name: "改名後の題名", ParentRoomID: &parentID},
		6: {ID: 6, Type: model.RoomTypeCommunity, Name: "写真部2"},
	}
	ctx := context.Background()

	old := &model.ReportedContent{RoomID: 5, RoomType: model.RoomTypeCommunity, RoomName: "合宿の相談"}
	got := r.withThreadParent(ctx, old)
	if got.RoomID != 4 || got.RoomName != "写真部" || got.ThreadTitle == nil || *got.ThreadTitle != "合宿の相談" {
		t.Fatalf("got %+v thread=%v; want the parent with the title at the time of the report", got, got.ThreadTitle)
	}
	if old.ThreadTitle != nil || old.RoomName != "合宿の相談" {
		t.Fatal("the stored snapshot was modified")
	}

	// スレッドでないルーム・消えたルーム・DM はそのまま。
	for _, m := range []*model.ReportedContent{
		{RoomID: 6, RoomType: model.RoomTypeCommunity, RoomName: "写真部2"},
		{RoomID: 99, RoomType: model.RoomTypeCommunity, RoomName: "消えたスレッド"},
		{RoomID: 5, RoomType: "dm", RoomName: ""},
	} {
		if got := r.withThreadParent(ctx, m); got != m {
			t.Errorf("%+v was rewritten to %+v", m, got)
		}
	}
}
