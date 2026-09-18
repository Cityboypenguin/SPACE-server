package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

func TestPostReplyCount_CreateDeleteAndUserDeletion(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_reply_count", []string{
		`CREATE TABLE posts (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			content TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			parent_id BIGINT NULL,
			deleted_at BIGINT NULL,
			reply_count INT NOT NULL DEFAULT 0,
			INDEX idx_posts_parent_id (parent_id),
			INDEX idx_posts_user_id (user_id)
		)`,
	})
	defer cleanup()

	repo := &MySQLPostRepository{DB: db}
	ctx := context.Background()
	create := func(userID int64, parentID *int64) int64 {
		t.Helper()
		now := time.Now()
		id, err := repo.CreatePost(ctx, &model.Post{
			Content: "test", UserID: userID, ParentID: parentID,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("CreatePost: %v", err)
		}
		return id
	}
	count := func(postID int64) int {
		t.Helper()
		var value int
		if err := db.QueryRowContext(ctx, `SELECT reply_count FROM posts WHERE id = ?`, postID).Scan(&value); err != nil {
			t.Fatalf("read reply_count: %v", err)
		}
		return value
	}

	rootID := create(1, nil)
	replyID := create(2, &rootID)
	nestedID := create(3, &replyID)
	_ = create(3, &rootID)
	if got := count(rootID); got != 3 {
		t.Fatalf("root reply_count after creates = %d, want 3", got)
	}
	if got := count(replyID); got != 1 {
		t.Fatalf("reply reply_count after creates = %d, want 1", got)
	}

	deleted, err := repo.DeletePost(ctx, nestedID)
	if err != nil || !deleted {
		t.Fatalf("DeletePost = %v, %v", deleted, err)
	}
	if got := count(rootID); got != 2 {
		t.Fatalf("root reply_count after nested delete = %d, want 2", got)
	}
	if got := count(replyID); got != 0 {
		t.Fatalf("reply reply_count after nested delete = %d, want 0", got)
	}

	if err := repo.DeletePostsByUserID(ctx, 2); err != nil {
		t.Fatalf("DeletePostsByUserID: %v", err)
	}
	if err := repo.RecalculateReplyCountsAffectedByUser(ctx, 2); err != nil {
		t.Fatalf("RecalculateReplyCountsAffectedByUser: %v", err)
	}
	if got := count(rootID); got != 1 {
		t.Fatalf("root reply_count after user deletion = %d, want 1", got)
	}
}
