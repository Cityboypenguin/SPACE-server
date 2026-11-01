package report

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const reportNotificationSubject = "【Senshu-Universe】通報を受け付けました"

// defaultReportNotificationWindow は、1通送ったあと次の1通までに空ける時間。
const defaultReportNotificationWindow = 5 * time.Minute

var reportNotificationZone = time.FixedZone("JST", 9*60*60)

var reportTargetLabels = map[model.ReportTargetType]string{
	model.TargetPost:      "投稿",
	model.TargetUser:      "ユーザー",
	model.TargetCommunity: "コミュニティ",
	model.TargetMessage:   "メッセージ",
	model.TargetQuestion:  "質問",
	model.TargetAnswer:    "回答",
	model.TargetPoll:      "投票",
}

// 通報モーダルの選択肢と対応する。reason は利用者が自由に送れる文字列なので、
// 知らない値はそのままメールへ載せず「その他」にする。
var reportReasonLabels = map[string]string{
	"SPAM":                "スパム / 宣伝目的",
	"HARASSMENT":          "嫌がらせ / 誹謗中傷",
	"INAPPROPRIATE":       "不適切なコンテンツ",
	"COMMUNITY_VIOLATION": "コミュニティガイドライン違反",
	"OTHER":               "その他",
}

// ReportCreatedNotifier は通報を受け付けたことを知らせる。
type ReportCreatedNotifier interface {
	Notify(ctx context.Context, report *model.Report)
}

var _ ReportCreatedNotifier = &EmailReportCreatedNotifier{}

// EmailReportCreatedNotifier は登録済みの宛先へお知らせメールを送る。
//
// メールは暗号化されない経路を通り、受信箱にも残るので、本文には通報された内容・
// 詳細説明・通報者を載せない。DM の本文を保存時に暗号化している意味が無くなるため。
// 中身は管理画面で確認してもらう。
//
// 送信はリクエストの外で行い、失敗しても通報そのものは成功のままにする
// （メールの不調で利用者の通報を失わせない）。
//
// 通報は誰でも連続して送れるので、1件ごとに送ると受信箱があふれ、OTP と共用している
// SMTP の送信枠も使い切ってしまう。そこで、静かなときの1件目はすぐに送り、それから
// window の間に届いた通報は件数だけ数えて、window の終わりに1通へまとめる。
// 宛先ごとのメールは window あたり最大1通になる。
//
// 数えている件数はプロセスのメモリにあるだけなので、複数台で動かすと台数ぶん送られ、
// 異常終了すると数えていたぶんのメールは失われる（通報そのものは DB に残る）。
type EmailReportCreatedNotifier struct {
	recipients repository.ReportNotificationRecipientRepository
	mailer     repository.Mailer
	async      *async.Runner
	window     time.Duration

	mu      sync.Mutex
	timer   *time.Timer // window の途中なら非 nil
	pending int         // window の間に届いて、まだ知らせていない通報の数
}

func NewEmailReportCreatedNotifier(recipients repository.ReportNotificationRecipientRepository, mailer repository.Mailer, runner *async.Runner) *EmailReportCreatedNotifier {
	return NewEmailReportCreatedNotifierWithWindow(recipients, mailer, runner, defaultReportNotificationWindow)
}

// NewEmailReportCreatedNotifierWithWindow is exposed for tests that cannot wait for the default window.
func NewEmailReportCreatedNotifierWithWindow(recipients repository.ReportNotificationRecipientRepository, mailer repository.Mailer, runner *async.Runner, window time.Duration) *EmailReportCreatedNotifier {
	return &EmailReportCreatedNotifier{recipients: recipients, mailer: mailer, async: runner, window: window}
}

func (n *EmailReportCreatedNotifier) Notify(ctx context.Context, report *model.Report) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		n.pending++
		return
	}
	n.timer = time.AfterFunc(n.window, n.closeWindow)
	n.sendAsync(ctx, reportNotificationSubject, reportNotificationBody(report))
}

// closeWindow は window の終わりに呼ばれる。まとめて知らせたなら、その1通からまた
// window を空ける。何も無ければ window を閉じ、次の1件目はすぐに送れる状態へ戻す。
func (n *EmailReportCreatedNotifier) closeWindow() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer == nil {
		return // Flush が先に片付けた
	}
	if n.pending == 0 {
		n.timer = nil
		return
	}
	count := n.pending
	n.pending = 0
	n.timer = time.AfterFunc(n.window, n.closeWindow)
	n.sendSummary(context.Background(), count)
}

// Flush は停止時に呼ぶ。数えている通報があれば待たずに知らせ、以後の window を止める。
// 送信は Runner に載るので、呼び出し側はこのあと Runner.Wait で待つこと。
func (n *EmailReportCreatedNotifier) Flush(ctx context.Context) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	if n.pending > 0 {
		n.sendSummary(ctx, n.pending)
		n.pending = 0
	}
}

func (n *EmailReportCreatedNotifier) sendSummary(ctx context.Context, count int) {
	subject := fmt.Sprintf("%s（%d件）", reportNotificationSubject, count)
	body := fmt.Sprintf(
		"前回のお知らせのあと、新しい通報を %d 件受け付けました。\n\n内容は管理画面の「通報管理」で確認してください。\n",
		count,
	)
	n.sendAsync(ctx, subject, body)
}

func (n *EmailReportCreatedNotifier) sendAsync(ctx context.Context, subject, body string) {
	n.async.Go(ctx, "report_created", func(ctx context.Context) {
		recipients, err := n.recipients.List(ctx)
		if err != nil {
			logger.Log.Error().Err(err).
				Str("component", "report_notification").
				Msg("failed to list report notification recipients")
			return
		}
		for _, recipient := range recipients {
			// 1人に届かなくても残りの宛先には送る。
			if err := n.mailer.Send(ctx, recipient.Email, subject, body); err != nil {
				logger.Log.Error().Err(err).
					Str("component", "report_notification").
					Int64("recipient_id", recipient.ID).
					Msg("failed to send a report notification")
			}
		}
	})
}

func reportNotificationBody(report *model.Report) string {
	target, ok := reportTargetLabels[report.TargetType]
	if !ok {
		target = "その他"
	}
	reason, ok := reportReasonLabels[report.Reason]
	if !ok {
		reason = "その他"
	}
	return fmt.Sprintf(
		"新しい通報を受け付けました。\n\n対象: %s\n理由: %s\n受付日時: %s\n通報ID: %s\n\n内容は管理画面の「通報管理」で確認してください。\n",
		target, reason, report.CreatedAt.In(reportNotificationZone).Format("2006/01/02 15:04"), report.ID,
	)
}
