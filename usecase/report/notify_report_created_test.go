package report

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeRecipientRepo struct {
	recipients []*model.ReportNotificationRecipient
	listErr    error
	createErr  error
	deleted    bool
}

func (f *fakeRecipientRepo) List(context.Context) ([]*model.ReportNotificationRecipient, error) {
	return f.recipients, f.listErr
}

func (f *fakeRecipientRepo) Create(_ context.Context, r *model.ReportNotificationRecipient) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.recipients = append(f.recipients, r)
	return nil
}

func (f *fakeRecipientRepo) Delete(context.Context, int64) (bool, error) { return f.deleted, nil }

type recordingMailer struct {
	mu       sync.Mutex
	to       []string
	subjects []string
	bodies   []string
	failTo   string
}

func (m *recordingMailer) Send(_ context.Context, to, subject, body string) error {
	if to == m.failTo {
		return errors.New("smtp down")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.to = append(m.to, to)
	m.subjects = append(m.subjects, subject)
	m.bodies = append(m.bodies, body)
	return nil
}

func (m *recordingMailer) sentSubjects() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.subjects...)
}

func waitForMails(t *testing.T, m *recordingMailer, want int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := m.sentSubjects(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("got %v, want %d mails", m.sentSubjects(), want)
	return nil
}

func notifyAndWait(t *testing.T, repo *fakeRecipientRepo, mailer *recordingMailer, report *model.Report) {
	t.Helper()
	runner := async.NewRunner("test")
	NewEmailReportCreatedNotifier(repo, mailer, runner).Notify(context.Background(), report)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestNotify_SendsToEveryRecipientWithoutReportContents(t *testing.T) {
	customReason := "詳細説明の秘密"
	content := "投稿本文の秘密"
	report := &model.Report{
		ID: "r1", ReporterID: 1, TargetType: model.TargetMessage, TargetID: "9", Reason: "HARASSMENT",
		CustomReason: &customReason, PostContent: &content, CreatedAt: time.Unix(1700000000, 0),
		ReportedMessage: &model.ReportedMessage{Content: "DM本文の秘密", AuthorName: "投稿者の名前"},
	}
	mailer := &recordingMailer{failTo: "bad@example.com"}
	repo := &fakeRecipientRepo{recipients: []*model.ReportNotificationRecipient{
		{ID: 1, Email: "bad@example.com"}, {ID: 2, Email: "ok@example.com"},
	}}
	notifyAndWait(t, repo, mailer, report)

	if len(mailer.to) != 1 || mailer.to[0] != "ok@example.com" {
		t.Fatalf("to = %v; a failed recipient must not stop the others", mailer.to)
	}
	body := mailer.bodies[0]
	for _, secret := range []string{"秘密", "投稿者の名前"} {
		if strings.Contains(body, secret) {
			t.Fatalf("body must not contain %q: %s", secret, body)
		}
	}
	for _, want := range []string{"メッセージ", "嫌がらせ / 誹謗中傷", "r1", "2023/11/15 07:13"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body must contain %q: %s", want, body)
		}
	}
}

func TestNotify_UnknownReasonIsNotCopiedIntoTheMail(t *testing.T) {
	mailer := &recordingMailer{}
	repo := &fakeRecipientRepo{recipients: []*model.ReportNotificationRecipient{{ID: 1, Email: "ok@example.com"}}}
	notifyAndWait(t, repo, mailer, &model.Report{ID: "r1", TargetType: model.TargetPost, Reason: "http://evil.example/ 至急確認"})
	if strings.Contains(mailer.bodies[0], "evil.example") {
		t.Fatalf("free-form reason leaked into the mail: %s", mailer.bodies[0])
	}
}

func TestNotify_CombinesReportsWithinTheWindow(t *testing.T) {
	window := 200 * time.Millisecond
	mailer := &recordingMailer{}
	repo := &fakeRecipientRepo{recipients: []*model.ReportNotificationRecipient{{ID: 1, Email: "ok@example.com"}}}
	runner := async.NewRunner("test")
	n := NewEmailReportCreatedNotifierWithWindow(repo, mailer, runner, window)
	ctx := context.Background()
	report := &model.Report{ID: "r", TargetType: model.TargetPost, Reason: "SPAM"}

	// 1件目はすぐに届く。
	n.Notify(ctx, report)
	if got := waitForMails(t, mailer, 1); got[0] != reportNotificationSubject {
		t.Fatalf("first subject = %q", got[0])
	}

	// window の途中の通報は、終わるまでメールにならない。
	for i := 0; i < 3; i++ {
		n.Notify(ctx, report)
	}
	if got := mailer.sentSubjects(); len(got) != 1 {
		t.Fatalf("mails during the window = %v", got)
	}

	// window の終わりに件数入りの1通へまとまる。
	got := waitForMails(t, mailer, 2)
	if !strings.Contains(got[1], "3件") {
		t.Fatalf("summary subject = %q", got[1])
	}
	if body := mailer.bodies[1]; !strings.Contains(body, "3 件") {
		t.Fatalf("summary body = %q", body)
	}

	// まとめの1通からも window を空けるので、直後の通報もすぐには送らない。
	n.Notify(ctx, report)
	if got := mailer.sentSubjects(); len(got) != 2 {
		t.Fatalf("mails right after the summary = %v", got)
	}
	if got := waitForMails(t, mailer, 3); !strings.Contains(got[2], "1件") {
		t.Fatalf("second summary subject = %q", got[2])
	}

	// 静かな window を1つ挟めば、次の1件目はまたすぐに届く。
	time.Sleep(2*window + 100*time.Millisecond)
	n.Notify(ctx, report)
	if got := waitForMails(t, mailer, 4); got[3] != reportNotificationSubject {
		t.Fatalf("subject after a quiet window = %q", got[3])
	}
}

func TestNotify_FlushSendsWhatIsPendingOnce(t *testing.T) {
	mailer := &recordingMailer{}
	repo := &fakeRecipientRepo{recipients: []*model.ReportNotificationRecipient{{ID: 1, Email: "ok@example.com"}}}
	runner := async.NewRunner("test")
	n := NewEmailReportCreatedNotifierWithWindow(repo, mailer, runner, time.Hour)
	ctx := context.Background()
	report := &model.Report{ID: "r", TargetType: model.TargetPost, Reason: "SPAM"}

	n.Notify(ctx, report)
	n.Notify(ctx, report)
	n.Notify(ctx, report)
	n.Flush(ctx)
	n.Flush(ctx)

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := runner.Wait(waitCtx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	got := mailer.sentSubjects()
	if len(got) != 2 || !strings.Contains(got[1], "2件") {
		t.Fatalf("mails = %v, want the first report and one summary of 2", got)
	}
}

func TestManageNotificationRecipient(t *testing.T) {
	admin := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"})
	student := auth.WithClaims(context.Background(), &auth.Claims{ID: 2, Role: "student"})

	repo := &fakeRecipientRepo{}
	uc := NewManageNotificationRecipientUsecase(repo)

	if _, err := uc.List(student); err == nil {
		t.Fatal("a non-admin must not list recipients")
	}
	if _, err := uc.Add(student, "a@example.com"); err == nil {
		t.Fatal("a non-admin must not add a recipient")
	}
	if err := uc.Remove(student, 1); err == nil {
		t.Fatal("a non-admin must not remove a recipient")
	}

	got, err := uc.Add(admin, "  Staff@Example.COM ")
	if err != nil || got.Email != "staff@example.com" {
		t.Fatalf("Add = %+v, %v", got, err)
	}

	for _, bad := range []string{"", "not-an-email", "a@example.com\r\nBcc: x@example.com", "名前 <a@example.com>", "a@example.com, b@example.com"} {
		if _, err := uc.Add(admin, bad); apperr.CodeOf(err) != apperr.CodeInvalidInput {
			t.Fatalf("Add(%q) err = %v, want invalid input", bad, err)
		}
	}

	repo.createErr = repository.ErrDuplicateKey
	if _, err := uc.Add(admin, "staff@example.com"); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("duplicate err = %v, want conflict", err)
	}

	if err := uc.Remove(admin, 99); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("missing remove err = %v, want not found", err)
	}
	repo.deleted = true
	if err := uc.Remove(admin, 1); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
