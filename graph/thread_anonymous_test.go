package graph

import (
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"testing"
)

func TestCourseThreadUsesParentAnonymousIdentity(t *testing.T) {
	parent := int64(1)
	f := newLoaderFixture(t, fixtureOpts{
		rooms:      map[int64]*model.Room{7: {ID: 7, Type: model.RoomTypeCourse, ParentRoomID: &parent}},
		identities: map[repository.RoomUserKey]*model.RoomAnonymousIdentity{{RoomID: parent, UserID: 42}: {ID: 99, RoomID: parent, UserID: 42, Label: "匿名007"}},
		users:      map[int64]*model.User{42: {ID: 42, Name: "実名"}},
	})
	r := &messageResolver{&Resolver{}}
	user, err := r.User(f.ctx, messageIn(7, 42))
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Name != "匿名007" {
		t.Fatalf("parent identity was not inherited: %+v", user)
	}
}
