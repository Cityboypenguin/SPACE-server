// Package video は投稿・メッセージに付ける動画の扱いをまとめる。
//
//   - 受け入れ（上限の確認・ファイルの検査・サムネイル）と保存
//   - 再生の可否と、期限付きの再生用URLの発行
//   - 保存期間の管理（期限の計算・変更の影響確認）
//   - 実体の削除（日次の処理・失敗時のやり直し）
//   - 量の把握（管理画面の集計）
//
// 方針は docs/video-posts.md にある。
package video

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// system_settings のキー。初期値はマイグレーション（133）が入れる。
// 値はコードに書かない（保存期間などは運用で変えるため）。
const (
	SettingRetentionDays         = "video_retention_days"
	SettingPurgeGraceDays        = "video_purge_grace_days"
	SettingPurgeBatchSize        = "video_purge_batch_size"
	SettingDailyUploadLimit      = "video_daily_upload_limit_per_user"
	SettingDailyBytesLimit       = "video_daily_bytes_limit_per_user"
	SettingTotalBytesLimit       = "video_total_bytes_limit"
	day                          = 24 * time.Hour
	backstopSafetyMarginDays     = 7
	maxRetentionDaysWithoutLimit = 3650
)

// Policy は動画の設定一式。毎回 DB から読む（管理画面で変えたらすぐ効かせるため）。
type Policy struct {
	RetentionDays  int
	PurgeGraceDays int
	PurgeBatchSize int
	// 以下の上限は 0 なら無制限。
	DailyUploadLimit int64
	DailyBytesLimit  int64
	TotalBytesLimit  int64
}

// Retention は保存期間。
func (p Policy) Retention() time.Duration { return time.Duration(p.RetentionDays) * day }

// PurgeGrace は見えなくしてから実体を消すまでの猶予。
func (p Policy) PurgeGrace() time.Duration { return time.Duration(p.PurgeGraceDays) * day }

// ErrVideoDisabled は設定が欠けていて動画を扱えないこと。
//
// 設定の行が無いときに既定値で動かすと、それはコードに保存期間を書いたのと同じに
// なる（運用で消した・入れ忘れたことに気づけないまま、その値で消していく）。
// 動画の投稿と再生を止めて、気づける形で失敗させる。
var ErrVideoDisabled = errors.New("動画の機能は現在使えません")

// LoadPolicy は system_settings から設定を読む。どれか1つでも欠けていたり壊れていれば
// ErrVideoDisabled を包んで返す。
func LoadPolicy(ctx context.Context, settings repository.SystemSettingRepository) (Policy, error) {
	var p Policy
	var err error
	read := func(key string, min int64) int64 {
		if err != nil {
			return 0
		}
		raw, getErr := settings.Get(ctx, key)
		if getErr != nil {
			err = getErr
			return 0
		}
		raw = strings.TrimSpace(raw)
		v, parseErr := strconv.ParseInt(raw, 10, 64)
		if raw == "" || parseErr != nil || v < min {
			err = fmt.Errorf("%w: system setting %s is missing or invalid (%q)", ErrVideoDisabled, key, raw)
			return 0
		}
		return v
	}
	p.RetentionDays = int(read(SettingRetentionDays, 1))
	p.PurgeGraceDays = int(read(SettingPurgeGraceDays, 0))
	p.PurgeBatchSize = int(read(SettingPurgeBatchSize, 1))
	p.DailyUploadLimit = read(SettingDailyUploadLimit, 0)
	p.DailyBytesLimit = read(SettingDailyBytesLimit, 0)
	p.TotalBytesLimit = read(SettingTotalBytesLimit, 0)
	if err != nil {
		return Policy{}, err
	}
	return p, nil
}

// MaxRetentionDays は設定できる保存期間の上限。
//
// Azure のライフサイクル管理（安全網）は、作られてから backstopDays 日を過ぎた動画を
// アプリと無関係に消す。保存期間＋猶予がそれに届くと、アプリがまだ見せるつもりの
// 動画を Azure が先に消してしまう。そうならないよう、猶予と余裕（7日）を引いた
// 日数までしか設定させない。backstopDays が 0（安全網を置いていない環境）なら
// 実質の上限は設けない。
func MaxRetentionDays(backstopDays, graceDays int) int {
	if backstopDays <= 0 {
		return maxRetentionDaysWithoutLimit
	}
	return backstopDays - graceDays - backstopSafetyMarginDays
}

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

// dayKey は t の JST の日付を yyyymmdd の数で返す（video_daily_stats の主キー）。
func dayKey(t time.Time) int {
	l := t.In(jst)
	return l.Year()*10000 + int(l.Month())*100 + l.Day()
}

// startOfDayJST は t を含む JST の日の始まり。
func startOfDayJST(t time.Time) time.Time {
	l := t.In(jst)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, jst)
}
