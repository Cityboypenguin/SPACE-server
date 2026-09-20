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
