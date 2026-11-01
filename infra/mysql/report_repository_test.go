package mysql

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 通報時点のメッセージは DM の本文を含むので、平文で列に残らないことと、
// 暗号化したものが読み戻せることを実物の MySQL で確かめる。
func TestReportRepository_ReportedContentIsEncryptedAndRoundTrips(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_report_test", []string{
		`CREATE TABLE user_reports (
			id VARCHAR(255) NOT NULL,
			target_type VARCHAR(50) NOT NULL,
			target_id VARCHAR(255) NOT NULL,
			reporter_id BIGINT NOT NULL,
			reason VARCHAR(100) NOT NULL,
			custom_reason TEXT,
			content TEXT NULL,
			message_snapshot MEDIUMTEXT NULL,
			status VARCHAR(50) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		) DEFAULT CHARSET=utf8mb4`,
	})
	defer cleanup()

	t.Setenv("MESSAGE_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	repo, err := NewMySQLReportRepository(db)
	if err != nil {
		t.Fatalf("NewMySQLReportRepository: %v", err)
	}

	ctx := context.Background()
	now := time.Unix(1700000000, 0)
	authorID := int64(2)
	body := "通報時点の本文 " + strings.Repeat("長", 21000) // TEXT の上限 64KB に収まらない長さになる
	report := &model.Report{
		ID: "r1", ReporterID: 1, TargetType: model.TargetMessage, TargetID: "9",
		Reason: "SPAM", Status: model.StatusPending, CreatedAt: now, UpdatedAt: now,
		ReportedContent: &model.ReportedContent{
			ContentID: 9, RoomID: 5, RoomType: model.RoomTypeDM, AuthorID: &authorID, AuthorName: "実名",
			Content:   body,
			Media:     []model.ReportedContentMedia{{ID: 3, StorageKey: "media/a.png", ContentType: "image/png", CreatedAt: now}},
			CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := repo.Save(ctx, report); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var stored string
	if err := db.QueryRowContext(ctx, `SELECT message_snapshot FROM user_reports WHERE id = 'r1'`).Scan(&stored); err != nil {
		t.Fatalf("read the raw column: %v", err)
	}
	if strings.Contains(stored, "通報時点の本文") || strings.Contains(stored, "実名") {
		t.Fatal("message_snapshot must not hold plaintext")
	}

	got, err := repo.FindByID(ctx, "r1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ReportedContent == nil || got.ReportedContent.Content != body {
		t.Fatal("snapshot did not round-trip through FindByID")
	}
	if got.ReportedContent.AuthorID == nil || *got.ReportedContent.AuthorID != 2 || len(got.ReportedContent.Media) != 1 {
		t.Fatalf("snapshot = %+v", got.ReportedContent)
	}

	items, total, err := repo.Search(ctx, nil, repository.PageQuery{Limit: 10, WithTotal: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ReportedContent == nil {
		t.Fatalf("Search = %d items, total %d", len(items), total)
	}
}
