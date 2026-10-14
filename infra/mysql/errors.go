package mysql

import (
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
	drivermysql "github.com/go-sql-driver/mysql"
)

// mysqlDuplicateEntryErrno は MySQL の「Duplicate entry ... for key ...」(ER_DUP_ENTRY)。
const mysqlDuplicateEntryErrno = 1062

// isDuplicateKeyError は err が一意制約違反かを判定する。errno の比較はここ1箇所に
// 集約し、他の infra/mysql のコードはこの関数（または wrapDuplicateKey）を使う。
func isDuplicateKeyError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == mysqlDuplicateEntryErrno
}

// wrapDuplicateKey は一意制約違反を repository.ErrDuplicateKey に包み替えて返す。
// それ以外のエラーと nil はそのまま返すので、INSERT の戻り値をそのまま通せる。
func wrapDuplicateKey(err error) error {
	if err != nil && isDuplicateKeyError(err) {
		return fmt.Errorf("%w: %v", repository.ErrDuplicateKey, err)
	}
	return err
}

// mysqlDeadlockErrno は MySQL の「Deadlock found when trying to get lock」(ER_LOCK_DEADLOCK)。
// InnoDB はトランザクションを1つ巻き戻して返すので、最初からやり直せば通る。
const mysqlDeadlockErrno = 1213

func isDeadlockError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == mysqlDeadlockErrno
}

// deadlockRetries はデッドロックで巻き戻されたトランザクションをやり直す回数の上限。
const deadlockRetries = 3

// retryOnDeadlock は fn（トランザクション1つ全体）をデッドロックのときだけやり直す。
// fn は毎回トランザクションを開き直すこと（巻き戻されたトランザクションは使えない）。
func retryOnDeadlock[T any](fn func() (T, error)) (T, error) {
	for attempt := 1; ; attempt++ {
		v, err := fn()
		if err == nil || !isDeadlockError(err) || attempt == deadlockRetries {
			return v, err
		}
	}
}
