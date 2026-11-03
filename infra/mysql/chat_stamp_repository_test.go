package mysql

import (
	"context"
	"os"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/usecase/chatstamp"
)

// Uses a throwaway schema, never the development application's tables.
func TestChatStampCatalogMigrationCASAndAtomicOrdering(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()
	mustExec(t, db, `ALTER TABLE messages ADD stamp_id VARCHAR(32) NULL`)
	mustExec(t, db, `CREATE TABLE message_reactions (emoji VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL)`)
	for _, name := range []string{"127_create_chat_stamps.up.sql", "128_widen_message_stamp_id.up.sql", "129_widen_message_reaction_id.up.sql"} {
		migration, e := os.ReadFile("../../db/migrations/" + name)
		if e != nil {
			t.Fatal(e)
		}
		// Execute each complete migration just like the application's driver.
		mustExec(t, db, string(migration))
	}

	repo := &ChatStampRepository{DB: db}
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "administrator"})
	svc := &chatstamp.Service{Repo: repo, Tx: NewMySQLTxManager(db)}
	a, e := svc.Save(ctx, model.ChatStamp{Label: "ありがとう", DisplaySize: "standard", Status: "draft", AllowMessage: true, AllowReaction: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := svc.Save(ctx, model.ChatStamp{Label: "了解", DisplaySize: "small", Status: "draft", AllowMessage: true, AllowReaction: false}, nil)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := repo.List(ctx)
	if e != nil || len(rows) != 2 {
		t.Fatalf("rows=%v err=%v", rows, e)
	}
	a.ImageKey = "chat-stamps/1/test.png"
	a.Status = "active"
	ok, e := repo.Update(ctx, a)
	if e != nil || !ok {
		t.Fatalf("update: %v %v", ok, e)
	}
	if ok, e = repo.Update(ctx, a); e != nil || ok {
		t.Fatalf("stale update: %v %v", ok, e)
	}
	a.Version++
	// Updating b first, then hitting a stale version on a, must roll b back.
	e = svc.Reorder(ctx, []chatstamp.OrderItem{{ID: b.ID, Version: b.Version}, {ID: a.ID, Version: 1}}, true)
	if e == nil {
		t.Fatal("stale reorder succeeded")
	}
	got, e := repo.Get(ctx, b.ID)
	if e != nil || got.Version != b.Version || got.ReactionOrder != 0 {
		t.Fatalf("partial reorder committed: %+v %v", got, e)
	}
	e = svc.Reorder(ctx, []chatstamp.OrderItem{{ID: b.ID, Version: b.Version}, {ID: a.ID, Version: a.Version}}, true)
	if e != nil {
		t.Fatal(e)
	}
	got, e = repo.Get(ctx, a.ID)
	if e != nil || got.ReactionOrder != 1 || got.MessageOrder != 0 {
		t.Fatalf("independent order: %+v %v", got, e)
	}
	// A generated ID exceeds the old VARCHAR(32); the migration must accept it.
	mustExec(t, db, `INSERT INTO message_reactions (emoji) VALUES (?)`, a.ID)
	if e := svc.RequireAllowed(ctx, a.ID, true); e != nil {
		t.Fatal(e)
	}
	got.Status = "inactive"
	ok, e = repo.Update(ctx, got)
	if e != nil || !ok {
		t.Fatal(e)
	}
	if e := svc.RequireAllowed(ctx, a.ID, true); e == nil {
		t.Fatal("stopped stamp accepted")
	}
	public, e := svc.List(ctx, false)
	if e != nil || len(public) != 1 || public[0].ID != a.ID {
		t.Fatalf("history not available or draft leaked: %+v %v", public, e)
	}
}
