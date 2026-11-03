package chatstamp

import (
	"context"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"testing"
)

type memoryStamps struct {
	repository.ChatStampRepository
	rows     map[string]*model.ChatStamp
	conflict bool
}

func (r *memoryStamps) Get(_ context.Context, id string) (*model.ChatStamp, error) {
	v := r.rows[id]
	if v == nil {
		return nil, nil
	}
	c := *v
	return &c, nil
}
func (r *memoryStamps) List(context.Context) ([]*model.ChatStamp, error) {
	out := []*model.ChatStamp{}
	for _, v := range r.rows {
		c := *v
		out = append(out, &c)
	}
	return out, nil
}
func (r *memoryStamps) Create(_ context.Context, v *model.ChatStamp) error {
	c := *v
	r.rows[v.ID] = &c
	return nil
}
func (r *memoryStamps) Update(_ context.Context, v *model.ChatStamp) (bool, error) {
	if r.conflict || r.rows[v.ID].Version != v.Version {
		return false, nil
	}
	c := *v
	c.Version++
	r.rows[v.ID] = &c
	return true, nil
}
func adminCtx() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 7, Role: "administrator"})
}
func userCtx() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 8, Role: "user"})
}
func fixture() (*Service, *memoryStamps) {
	r := &memoryStamps{rows: map[string]*model.ChatStamp{}}
	return &Service{Repo: r}, r
}
func draft() model.ChatStamp {
	return model.ChatStamp{Label: "ありがとう", DisplaySize: "standard", Status: "draft", AllowMessage: true, AllowReaction: true}
}
func TestAdminOnlyAndDraftFirst(t *testing.T) {
	s, _ := fixture()
	if _, e := s.Save(userCtx(), draft(), nil); apperr.CodeOf(e) != apperr.CodeForbidden {
		t.Fatalf("non-admin save: %v", e)
	}
	if _, e := s.List(context.Background(), false); apperr.CodeOf(e) != apperr.CodeUnauthorized {
		t.Fatalf("anonymous list: %v", e)
	}
	v := draft()
	v.Status = "active"
	if _, e := s.Save(adminCtx(), v, nil); e == nil {
		t.Fatal("created as active")
	}
	v = draft()
	v.ImageKey = "forged/key"
	got, e := s.Save(adminCtx(), v, nil)
	if e != nil {
		t.Fatal(e)
	}
	if got.ImageKey != "" {
		t.Fatal("accepted forged image key")
	}
	rows, e := s.List(userCtx(), false)
	if e != nil || len(rows) != 0 {
		t.Fatalf("draft leaked: %v %v", rows, e)
	}
}
func TestUsageFlagsAndStoppedHistory(t *testing.T) {
	s, r := fixture()
	r.rows["stamp:a"] = &model.ChatStamp{ID: "stamp:a", Status: "active", AllowMessage: true, AllowReaction: false}
	if e := s.RequireAllowed(userCtx(), "stamp:a", false); e != nil {
		t.Fatal(e)
	}
	if e := s.RequireAllowed(userCtx(), "stamp:a", true); e == nil {
		t.Fatal("message-only stamp allowed as reaction")
	}
	r.rows["stamp:a"].Status = "inactive"
	if e := s.RequireAllowed(userCtx(), "stamp:a", false); e == nil {
		t.Fatal("inactive stamp allowed")
	}
	rows, e := s.List(userCtx(), false)
	if e != nil || len(rows) != 1 {
		t.Fatalf("inactive history unavailable: %v", e)
	}
	if e := s.RequireAllowed(userCtx(), "stamp:missing", false); e == nil {
		t.Fatal("unknown ID allowed")
	}
}
func TestConcurrentUpdatesAndPublishRequirements(t *testing.T) {
	s, r := fixture()
	v, e := s.Save(adminCtx(), draft(), nil)
	if e != nil {
		t.Fatal(e)
	}
	v.Status = "active"
	if _, e := s.Save(adminCtx(), *v, nil); e == nil {
		t.Fatal("published without image")
	}
	r.rows[v.ID].ImageKey = "chat-stamps/7/existing.png"
	saved, e := s.Save(adminCtx(), *v, nil)
	if e != nil {
		t.Fatal(e)
	}
	if saved.Version != 2 {
		t.Fatal("version not advanced")
	}
	if _, e = s.Save(adminCtx(), *v, nil); e == nil {
		t.Fatal("stale save accepted")
	}
	saved.Status = "draft"
	if _, e = s.Save(adminCtx(), *saved, nil); e == nil {
		t.Fatal("published stamp made draft and hidden from history")
	}
	saved.Status = "inactive"
	r.conflict = true
	if _, e = s.Save(adminCtx(), *saved, nil); e == nil {
		t.Fatal("CAS conflict ignored")
	}
}
