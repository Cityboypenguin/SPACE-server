package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/redis/go-redis/v9"
)

// courseImportLockTTL は「実行中」の印の寿命。
//
// 印を持ったまま台が落ちると、誰も取り込んでいないのに「既に実行中です」と
// 言い続ける状態になる。寿命を付けておけば、放置されてもこの時間で解ける。
//
// 進捗の報告のたびに延びる（Update）。取り込みは進捗を報告しながら進むので、
// 生きている限り印が切れることはない。値は progressStallTimeout（10分、
// 取り込みが「固まった」と判断する時間）より長くしてある。短くすると、
// 遅いだけでまだ生きている取り込みの印が先に切れて、別の台が二重に始めてしまう。
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
	// owner はこの台が持っている印の合言葉。他の台が持っている印を
	// 取り違えて消さないために使う（寿命で解けた直後に別の台が取った印を、
	// 遅れて終わったこちらが消してしまう、という取り違えが起きうる）。
	owner string
}

var _ courseimport.Store = (*CourseImportStore)(nil)

func NewCourseImportStore(client *redis.Client, owner string) *CourseImportStore {
	return &CourseImportStore{client: client, owner: owner}
}

func (s *CourseImportStore) TryStart(ctx context.Context, status courseimport.Status) (bool, error) {
	ok, err := s.client.SetNX(ctx, courseImportLockKey, s.owner, courseImportLockTTL).Result()
	if err != nil {
		return false, fmt.Errorf("failed to take the course import lock: %w", err)
	}
	if !ok {
		return false, nil
	}
	if err := s.save(ctx, status); err != nil {
		// 印は取れたが状態を書けなかった。持ったままにすると誰も始められなく
		// なるので、その場で返す。
		_ = s.release(ctx)
		return false, err
	}
	return true, nil
}

func (s *CourseImportStore) Update(ctx context.Context, status courseimport.Status) error {
	// 印を持っているのが自分だと確かめてから延ばす。自分のものでなければ
	// 延ばさない（寿命で解けたあと別の台が取り込みを始めている場合に、
	// こちらが相手の印を延ばしてしまわないように）。
	held, err := s.holdsLock(ctx)
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	if err := s.client.Expire(ctx, courseImportLockKey, courseImportLockTTL).Err(); err != nil {
		return fmt.Errorf("failed to extend the course import lock: %w", err)
	}
	return s.save(ctx, status)
}

func (s *CourseImportStore) Finish(ctx context.Context, status courseimport.Status) error {
	if err := s.save(ctx, status); err != nil {
		return err
	}
	return s.release(ctx)
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

func (s *CourseImportStore) holdsLock(ctx context.Context) (bool, error) {
	owner, err := s.client.Get(ctx, courseImportLockKey).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the course import lock: %w", err)
	}
	return owner == s.owner, nil
}

// releaseScript は「自分が持っている印だけを消す」。
//
// GET してから DEL するのでは、その隙間で寿命が切れて別の台が取った印を
// 消してしまう。Redis 側で一続きに実行する必要がある。
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

func (s *CourseImportStore) release(ctx context.Context) error {
	if err := releaseScript.Run(ctx, s.client, []string{courseImportLockKey}, s.owner).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to release the course import lock: %w", err)
	}
	return nil
}
