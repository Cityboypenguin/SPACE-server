package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// courseImportLockTTL は「実行中」の印の寿命。
//
// 印を持ったまま台が落ちると、誰も取り込んでいないのに「既に実行中です」と
// 言い続ける状態になる。寿命を付けておけば、放置されてもこの時間で解ける。
// つまりこの値が意味するのは「台が死んでから、次の実行を始められるまでの待ち」
// ただ1つ。
//
// 生きている間は courseimport.LockHeartbeatInterval ごとに延びる。以前は進捗の
// 報告（Update）でしか延びなかったので、報告するものが無い区間――スクレイピングを
// 終えた後の一括保存――が長引くと、まだDBへ書いている実行を残したまま印が解けた。
// そのため値は「取り込みが固まったと判断する時間」より長く、という別の条件にも
// 縛られていた。心拍を入れたことでその縛りは外れている。
//
// 必要なのは心拍の間隔より十分長いことだけ（何回か落としても解けない程度）。
// その関係は TestCourseImportLockOutlivesSeveralHeartbeats が見張っている。
const courseImportLockTTL = 15 * time.Minute

const (
	courseImportStatusKey = "courseimport:status"
	courseImportLockKey   = "courseimport:running"
)

// CourseImportStore は授業インポートの状態を Redis に置く courseimport.Store。
//
// 排他は SET NX で取る（台をまたいで不可分）。状態そのものは別のキーに置き、
// 寿命を付けない。取り込みが終わったあとも「前回いつ何件入ったか」を
// 管理画面に出し続けるため。
type CourseImportStore struct {
	client *redis.Client
	// owner はこの台のID。印の合言葉そのものではなく、その頭に付ける目印
	// （ログや redis-cli から「どの台が持っているか」が読めるように）。
	//
	// 合言葉を台のIDにしてはいけない。印が寿命で解けた後、同じ台で次の実行が
	// 始まると、古い実行の報告も台のIDでは一致してしまい、新しい実行の状態を
	// 上書きできてしまう。合言葉は実行ごとに発行する（TryStart）。
	owner string
}

var _ courseimport.Store = (*CourseImportStore)(nil)

func NewCourseImportStore(client *redis.Client, owner string) *CourseImportStore {
	return &CourseImportStore{client: client, owner: owner}
}

func (s *CourseImportStore) TryStart(ctx context.Context, status courseimport.Status) (string, bool, error) {
	// 合言葉は実行ごとに新しく作る。台のIDだけを頭に付けておくのは、
	// 印を覗いたときにどの台が持っているか分かるようにするため。
	token := s.owner + ":" + uuid.NewString()

	ok, err := s.client.SetNX(ctx, courseImportLockKey, token, courseImportLockTTL).Result()
	if err != nil {
		return "", false, fmt.Errorf("failed to take the course import lock: %w", err)
	}
	if !ok {
		return "", false, nil
	}
	if err := s.save(ctx, status); err != nil {
		// 印は取れたが状態を書けなかった。持ったままにすると誰も始められなく
		// なるので、その場で返す。
		_ = s.release(ctx, token)
		return "", false, err
	}
	return token, true, nil
}

func (s *CourseImportStore) Update(ctx context.Context, token string, status courseimport.Status) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to encode the course import status: %w", err)
	}

	// 「印の持ち主が自分か」「延長」「状態の保存」を一続きに行う
	// （courseImportUpdateScript のコメント参照）。
	held, err := courseImportUpdateScript.Run(ctx, s.client,
		[]string{courseImportLockKey, courseImportStatusKey},
		token, courseImportLockTTL.Milliseconds(), raw,
	).Bool()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to extend the course import lock: %w", err)
	}
	if !held {
		logCourseImportLockLost("update")
	}
	return nil
}

func (s *CourseImportStore) Finish(ctx context.Context, token string, status courseimport.Status) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to encode the course import status: %w", err)
	}

	// 終了も「自分が持っている間だけ」。保存してから解除する2段だと、寿命で解けて
	// 別の台が取り込みを始めた後に、こちらの結果（成功／失敗）でその実行中の状態を
	// 上書きしてしまう。管理画面には「終わりました」と出るのに実際はまだ走っている、
	// という食い違いになる。
	held, err := courseImportFinishScript.Run(ctx, s.client,
		[]string{courseImportLockKey, courseImportStatusKey},
		token, raw,
	).Bool()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to record the course import status: %w", err)
	}
	if !held {
		// 既に自分の印ではない＝この実行は寿命切れで見放され、別の台が引き継いで
		// いる（かもしれない）。結果を書かずに黙って終える。エラーにしないのは、
		// 取り込み自体は本当に終わっており、呼び出し側にできることが無いため。
		logCourseImportLockLost("finish")
	}
	return nil
}

// logCourseImportLockLost は「自分の印ではなくなっていた」を残す。
//
// 起きるのは、取り込みが courseImportLockTTL の間まったく進捗を報告できなかった
// ときだけ。正常系では起きないので、起きたら値が短すぎる疑いがある。
func logCourseImportLockLost(stage string) {
	logger.Log.Warn().
		Str("component", "redis_course_import").
		Str("stage", stage).
		Msg("the course import lock is no longer held by this instance; not writing the status")
}

// Heartbeat は自分が持っている間だけ印を延ばす。
//
// 進捗の報告（Update）と分けてあるのは、延ばすためだけに状態を書き直したくない
// から。取り込みが「報告するものが無いまま長く走る」区間でも、ここが動いている
// 限り印は解けない。
func (s *CourseImportStore) Heartbeat(ctx context.Context, token string) (bool, error) {
	held, err := courseImportHeartbeatScript.Run(ctx, s.client,
		[]string{courseImportLockKey}, token, courseImportLockTTL.Milliseconds(),
	).Bool()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("failed to extend the course import lock: %w", err)
	}
	return held, nil
}

func (s *CourseImportStore) Load(ctx context.Context) (courseimport.Status, error) {
	raw, err := s.client.Get(ctx, courseImportStatusKey).Bytes()
	if errors.Is(err, redis.Nil) {
		// まだ一度も取り込んでいない。
		return courseimport.Status{State: courseimport.StateIdle}, nil
	}
	if err != nil {
		return courseimport.Status{}, fmt.Errorf("failed to read the course import status: %w", err)
	}

	var status courseimport.Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return courseimport.Status{}, fmt.Errorf("failed to parse the course import status: %w", err)
	}

	// 「実行中」と書いてあるのに印が無いなら、走らせていた台が落ちている。
	// そのままにすると管理画面が永遠に実行中を出し続けるので、失敗として見せる
	// （印には寿命があるので、この状態は最大 courseImportLockTTL で現れる）。
	if status.State == courseimport.StateRunning {
		exists, err := s.client.Exists(ctx, courseImportLockKey).Result()
		if err != nil {
			return courseimport.Status{}, fmt.Errorf("failed to check the course import lock: %w", err)
		}
		if exists == 0 {
			status.State = courseimport.StateFailed
			status.ErrorMessage = "インポートを実行していたサーバーが停止したため、状態を確認できません"
		}
	}
	return status, nil
}

func (s *CourseImportStore) save(ctx context.Context, status courseimport.Status) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to encode the course import status: %w", err)
	}
	// 寿命を付けない。取り込みが終わったあとも前回の結果を出し続けるため。
	if err := s.client.Set(ctx, courseImportStatusKey, raw, 0).Err(); err != nil {
		return fmt.Errorf("failed to record the course import status: %w", err)
	}
	return nil
}

// 印に触る操作は全て Lua で一続きに行う。
//
// 「自分が持っているか確かめる」と「それを前提に何かする」を別々に撃つと、
// その隙間で寿命が切れて別の台が印を取りうる。そうなると、
//
//   - Update: 相手の印を延ばし、相手の実行中の状態を自分の進捗で上書きする
//   - Finish: 相手の実行中の状態を、自分の結果（成功／失敗）で上書きする
//   - release: 相手の印を消し、三台目が同時に始められるようにしてしまう
//
// のいずれかになる。どれも「取り込みが同時に2つ走る」か「管理画面が嘘の状態を
// 出す」に繋がる。Redis はスクリプトを1つずつ実行するので、確認と更新を
// この中へ入れてしまえば隙間が無くなる。

// courseImportUpdateScript は自分が印を持っているときだけ、印を延ばして状態を書く。
var courseImportUpdateScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
	return 0
end
redis.call("PEXPIRE", KEYS[1], ARGV[2])
redis.call("SET", KEYS[2], ARGV[3])
return 1
`)

// courseImportHeartbeatScript は自分が印を持っているときだけ寿命を延ばす。
var courseImportHeartbeatScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
	return 0
end
redis.call("PEXPIRE", KEYS[1], ARGV[2])
return 1
`)

// courseImportFinishScript は自分が印を持っているときだけ、結果を書いて印を外す。
var courseImportFinishScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
	return 0
end
redis.call("SET", KEYS[2], ARGV[2])
redis.call("DEL", KEYS[1])
return 1
`)

// releaseScript は「自分が持っている印だけを消す」。
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

func (s *CourseImportStore) release(ctx context.Context, token string) error {
	if err := releaseScript.Run(ctx, s.client, []string{courseImportLockKey}, token).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to release the course import lock: %w", err)
	}
	return nil
}
