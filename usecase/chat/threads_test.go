package chat

import (
	"context"
	"errors"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
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

func (s *threadRoomStore) GetRoomByID(_ context.Context, id int64) (*model.Room, error) {
	return s.rooms[id], nil
}

func (s *threadRoomStore) DeleteRoom(_ context.Context, id int64) (bool, error) {
	_, ok := s.rooms[id]
	delete(s.rooms, id)
	return ok, nil
}

type threadStore struct {
	repository.ChatThreadRepository
	rooms        map[int64]*model.Room
	created      bool
	updated      bool
	unreadCalled bool
	announcement int64
}

func (s *threadStore) AnnouncementID(_ context.Context, roomID int64) (int64, error) {
	return s.announcement, nil
}

func (s *threadStore) HasUnread(_ context.Context, parentID, viewerID int64) (bool, error) {
	s.unreadCalled = true
	return true, nil
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
	message     *model.Message
	err         error
	softDeleted []int64
}

func (s *threadMessageStore) SoftDeleteMessage(_ context.Context, id, deletedBy int64) (bool, error) {
	s.softDeleted = append(s.softDeleted, id)
	return true, nil
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
	s := &ThreadService{Access: h.access, Rooms: &threadRoomStore{rooms: rooms}, Threads: store, Tx: tx, Writers: MessageWriters{system: messages}, Role: &fakeRoomUserRole{}, Events: h.events}
	return s, h, store, messages, tx
}
func TestThreadCreateAnnouncement(t *testing.T) {
	for _, parentID := range []int64{communityRoomID, courseRoomID} {
		t.Run(string(rune('0'+parentID)), func(t *testing.T) {
			s, _, store, messages, tx := threadHarness()
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

func TestCourseThreadSendKeepsRealAuthorAndChildMessages(t *testing.T) {
	s, h, _, _, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), courseRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	message, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: thread.Room.ID, Content: "返信"})
	if err != nil {
		t.Fatal(err)
	}
	if message.RoomID != thread.Room.ID || message.UserID != 10 {
		t.Fatal("thread posts must stay in the child room and retain the real author")
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

func TestThreadUnreadRequiresParentReadAccess(t *testing.T) {
	s, _, store, _, _ := threadHarness()
	if _, err := s.HasUnread(ctxAsUser(99), communityRoomID); err == nil || store.unreadCalled {
		t.Fatal("nonmember can inspect thread unread state")
	}
	unread, err := s.HasUnread(ctxAsUser(10), communityRoomID)
	if err != nil || !unread || !store.unreadCalled {
		t.Fatal("parent member cannot see thread unread state", err)
	}
}

func TestThreadTitleIsScreenedOnlyInCourseRooms(t *testing.T) {
	s, _, _, _, _ := threadHarness()
	s.NGWords = &fakeNGGuard{contains: "NG"}

	if _, err := s.Create(ctxAsUser(10), courseRoomID, "NG の相談"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("course room: err = %v, want InvalidInput", err)
	}
	if _, err := s.Create(ctxAsUser(10), communityRoomID, "NG の相談"); err != nil {
		t.Errorf("community room: err = %v, want nil", err)
	}
	created, err := s.Create(ctxAsUser(10), courseRoomID, "課題の相談")
	if err != nil {
		t.Fatal(err)
	}
	renamed := "NG に変更"
	if _, err := s.Update(ctxAsUser(10), created.Room.ID, &renamed, nil); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Errorf("rename: err = %v, want InvalidInput", err)
	}
}

type fakeAnnouncementFinder struct {
	id  int64
	err error
}

func (f fakeAnnouncementFinder) AnnouncementID(context.Context, int64) (int64, error) {
	return f.id, f.err
}

func TestRefreshThreadCard_PublishesTheAnnouncementToTheParent(t *testing.T) {
	parentID := int64(communityRoomID)
	thread := &model.Room{ID: 50, ParentRoomID: &parentID}
	build := func(finder ThreadAnnouncementFinder) (*messageCommandService, *fakeEvents) {
		events := &fakeEvents{}
		return &messageCommandService{deps: MessageCommandDeps{
			Threads:    finder,
			GetMessage: &fakeGetMessage{msg: &model.Message{ID: 7, RoomID: parentID, UserID: 10}},
			GetRoom:    &fakeGetRoom{rooms: map[int64]*model.Room{parentID: {ID: parentID}}},
			Events:     events,
		}}, events
	}

	s, events := build(fakeAnnouncementFinder{id: 7})
	s.refreshThreadCard(context.Background(), thread)
	if len(events.updated) != 1 || events.updated[0].Message.ID != 7 || events.updated[0].Room.ID != parentID {
		t.Fatalf("updated = %+v", events.updated)
	}

	// スレッドでないルームでは何も配信しない。
	s, events = build(fakeAnnouncementFinder{id: 7})
	s.refreshThreadCard(context.Background(), &model.Room{ID: parentID})
	if len(events.updated) != 0 {
		t.Errorf("not a thread: updated = %+v", events.updated)
	}

	// カードが引けなくても発言は成功のまま（配信だけ諦める）。
	s, events = build(fakeAnnouncementFinder{err: errors.New("down")})
	s.refreshThreadCard(context.Background(), thread)
	if len(events.updated) != 0 {
		t.Errorf("lookup failed: updated = %+v", events.updated)
	}
}

func TestThreadDeleteRemovesRoomAndAnnouncement(t *testing.T) {
	s, h, store, messages, tx := threadHarness()
	thread, err := s.Create(ctxAsUser(10), communityRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	store.announcement = 77
	tx.called = false
	room, err := s.Delete(ctxAsAdmin(99), thread.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if room.ID != thread.Room.ID || !tx.called {
		t.Fatal("thread must be deleted in a transaction", room)
	}
	if _, ok := store.rooms[thread.Room.ID]; ok {
		t.Fatal("thread room was not deleted")
	}
	if len(messages.softDeleted) != 1 || messages.softDeleted[0] != 77 {
		t.Fatalf("announcement was not removed: %v", messages.softDeleted)
	}
	events := h.events.deleted
	if len(events) != 1 || events[0].RoomID != communityRoomID || events[0].MessageID != 77 {
		t.Fatalf("parent room was not told about the removed announcement: %+v", events)
	}
}

func TestThreadDeleteRejected(t *testing.T) {
	s, _, store, messages, _ := threadHarness()
	thread, err := s.Create(ctxAsUser(10), communityRoomID, "相談")
	if err != nil {
		t.Fatal(err)
	}
	// スレッドを作った本人やコミュニティのオーナーでも、消せるのは管理者だけ。
	if _, err := s.Delete(ctxAsUser(10), thread.Room.ID); err == nil {
		t.Fatal("thread creator deleted the thread")
	}
	// 親のルーム（スレッドでないルーム）は、この経路では消せない。
	if _, err := s.Delete(ctxAsAdmin(99), communityRoomID); err == nil {
		t.Fatal("parent room was deleted as a thread")
	}
	if _, err := s.Delete(ctxAsAdmin(99), 12345); err == nil {
		t.Fatal("missing thread was reported as deleted")
	}
	if _, ok := store.rooms[thread.Room.ID]; !ok || len(messages.softDeleted) != 0 {
		t.Fatal("rejected delete must not write")
	}
}
