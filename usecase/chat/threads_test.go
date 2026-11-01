package chat

import (
	"context"
	"errors"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"strings"
	"testing"
)

type threadRoomStore struct {
	repository.RoomRepository
	rooms map[int64]*model.Room
}

func (s *threadRoomStore) SaveRoom(_ context.Context, r *model.Room) error {
	r.ID = 42
	s.rooms[r.ID] = r
	return nil
}

type threadStore struct {
	repository.ChatThreadRepository
	rooms   map[int64]*model.Room
	created bool
	updated bool
}

func (s *threadStore) Create(_ context.Context, id, user int64) error { s.created = true; return nil }
func (s *threadStore) Get(_ context.Context, id, user int64) (*model.ChatThread, error) {
	r := s.rooms[id]
	if r == nil {
		return nil, nil
	}
	return &model.ChatThread{Room: r}, nil
}
func (s *threadStore) Update(_ context.Context, id int64, title string, closed bool) error {
	s.updated = true
	s.rooms[id].Name = title
	if closed {
		v := int64(1)
		s.rooms[id].ThreadClosedAt = &v
	} else {
		s.rooms[id].ThreadClosedAt = nil
	}
	return nil
}

type threadMessageStore struct {
	repository.MessageWriter
	message *model.Message
	err     error
}

func (s *threadMessageStore) SaveMessage(_ context.Context, m *model.Message) error {
	s.message = m
	return s.err
}

type threadTx struct{ called bool }

func (s *threadTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	s.called = true
	return fn(ctx)
}
func threadHarness() (*ThreadService, *harness, *threadStore, *threadMessageStore, *threadTx) {
	h := newHarness()
	rooms := h.accessDeps.GetRoom.(*fakeGetRoom).rooms
	store := &threadStore{rooms: rooms}
	messages := &threadMessageStore{}
	tx := &threadTx{}
	s := &ThreadService{Access: h.access, Rooms: &threadRoomStore{rooms: rooms}, Threads: store, Tx: tx, Writers: MessageWriters{system: messages}, Role: &fakeRoomUserRole{}, Anonymous: h.anonIdentity, Events: h.events}
	return s, h, store, messages, tx
}
func TestThreadCreateAnnouncement(t *testing.T) {
	for _, parentID := range []int64{communityRoomID, courseRoomID} {
		t.Run(string(rune('0'+parentID)), func(t *testing.T) {
			s, h, store, messages, tx := threadHarness()
			result, err := s.Create(ctxAsUser(10), parentID, "  課題の相談  ")
			if err != nil {
				t.Fatal(err)
			}
			if !tx.called || !store.created || messages.message == nil {
				t.Fatal("thread and announcement must be saved in a transaction")
			}
			if result.Room.Name != "課題の相談" || result.Room.PolicyRoomID() != parentID {
				t.Fatalf("unexpected thread %+v", result.Room)
			}
			msg := messages.message
			if msg.RoomID != parentID || msg.ThreadRoomID == nil || *msg.ThreadRoomID != result.Room.ID {
				t.Fatalf("incorrect announcement link: %+v", msg)
			}
			if parentID == courseRoomID && h.anonIdentity.calls != 1 {
				t.Fatal("course identity must be assigned on the parent")
			}
		})
	}
}
func TestThreadCreateRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		parent int64
		user   int64
		title  string
	}{
		{"DM", dmRoomID, 10, "相談"}, {"nonmember", communityRoomID, 99, "相談"}, {"blank", communityRoomID, 10, "  "}, {"long", communityRoomID, 10, strings.Repeat("あ", 101)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _, store, msg, tx := threadHarness()
			if _, err := s.Create(ctxAsUser(tt.user), tt.parent, tt.title); err == nil {
				t.Fatal("expected rejection")
			}
			if store.created || msg.message != nil || tx.called {
				t.Fatal("rejected request must not write")
			}
		})
	}
}
func TestThreadInheritsParentAccessAndIndependentReadPosition(t *testing.T) {
	s, h, _, _, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), communityRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	id := thread.Room.ID
	if _, err := h.access.EnsureWriteAccess(ctxAsUser(11), id); err != nil {
		t.Fatal("parent member must be able to write", err)
	}
	if _, err := h.access.EnsureReadAccess(ctxAsUser(99), id); err == nil {
		t.Fatal("nonmember can read thread")
	}
	if err := h.reads.MarkAsRead(ctxAsUser(11), id, nil); err != nil {
		t.Fatal(err)
	}
	if h.markRead.calls != 0 || h.markCourse.calls != 1 {
		t.Fatal("thread must have an independent upsertable read position")
	}
	h.members.byRoom[communityRoomID] = []int64{10}
	if _, err := h.access.EnsureReadAccess(ctxAsUser(11), id); err == nil {
		t.Fatal("removed member can read thread")
	}
	if err := h.reads.MarkAsRead(ctxAsUser(11), id, nil); err == nil {
		t.Fatal("removed member can mark thread read")
	}
	if _, err := s.Create(ctxAsUser(10), id, "nested"); err == nil {
		t.Fatal("nested thread was accepted")
	}
}
func TestThreadCloseAndRenamePermissions(t *testing.T) {
	s, h, store, _, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), communityRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	name := "変更後"
	if _, err = s.Update(ctxAsUser(11), thread.Room.ID, &name, &closed); err == nil || store.updated {
		t.Fatal("other member can modify thread")
	}
	if _, err = s.Update(ctxAsUser(10), thread.Room.ID, &name, &closed); err != nil {
		t.Fatal(err)
	}
	if _, err = h.access.EnsureWriteAccess(ctxAsUser(10), thread.Room.ID); err == nil {
		t.Fatal("closed thread accepts messages")
	}
	if _, err = h.access.EnsureReadAccess(ctxAsUser(11), thread.Room.ID); err != nil {
		t.Fatal("closed thread must remain readable", err)
	}
	closed = false
	if _, err = s.Update(ctxAsUser(10), thread.Room.ID, nil, &closed); err != nil {
		t.Fatal(err)
	}
	if _, err = h.access.EnsureWriteAccess(ctxAsUser(11), thread.Room.ID); err != nil {
		t.Fatal("reopened thread is not writable", err)
	}
}
func TestCourseThreadKeepsParentRestrictions(t *testing.T) {
	s, h, _, _, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), courseRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	h.checkWritable.err = errors.New("archived or not registered")
	if _, err = h.access.EnsureWriteAccess(ctxAsUser(10), thread.Room.ID); err == nil {
		t.Fatal("thread bypasses course write policy")
	}

}

func TestCourseThreadSendKeepsParentIdentityAndChildMessages(t *testing.T) {
	s, h, _, _, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), courseRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	message, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: thread.Room.ID, Content: "返信"})
	if err != nil {
		t.Fatal(err)
	}
	if message.RoomID != thread.Room.ID || h.anonIdentity.roomID != courseRoomID {
		t.Fatal("thread posts must stay in the child room while using the parent's identity")
	}
}
func TestThreadAnnouncementCannotBeEditedOrDeleted(t *testing.T) {
	s, h, _, messages, _ := threadHarness()
	_, err := s.Create(ctxAsUser(10), communityRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	h.getMessage.msg = messages.message
	if _, err = h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: communityRoomID, Content: "改変"}); err == nil {
		t.Fatal("system announcement was editable")
	}
	if _, err = h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: communityRoomID}); err == nil {
		t.Fatal("system announcement was deletable")
	}
	if h.updateMessage.called || h.deleteMessage.called {
		t.Fatal("system announcement reached a mutation writer")
	}
}
