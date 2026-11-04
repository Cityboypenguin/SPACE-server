package graph

import (
	"github.com/Cityboypenguin/SPACE-server/model"
	"testing"
)

func TestCourseThreadShowsRealAuthor(t *testing.T) {
	parent := int64(1)
	f := newLoaderFixture(t, fixtureOpts{
		rooms: map[int64]*model.Room{7: {ID: 7, Type: model.RoomTypeCourse, ParentRoomID: &parent}},
		users: map[int64]*model.User{42: realUser(42, "山田太郎")},
	})
	r := &messageResolver{&Resolver{}}
	msg := messageIn(7, 42)
	user, err := r.User(f.ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Name != "山田太郎" || user.ID != encodeGraphID("user", 42) {
		t.Fatalf("thread must show the real author: %+v", user)
	}
	id, err := r.UserID(f.ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if id != user.ID {
		t.Fatal("thread author ID and display user must match")
	}
}
