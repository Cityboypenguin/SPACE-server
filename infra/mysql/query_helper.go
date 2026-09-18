// infra/mysql/query_helper.go
package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// AppendBlockFilter は、対象のユーザーカラムにブロック除外 (NOT IN) 条件を安全に追加します。
// ※注意: baseQuery は WHERE 句の途中（ORDER BY や LIMIT の前）である必要があります。
//
// ブロック一覧をこのリクエストで取得できていない場合はエラーを返す。呼び出し側は
// そのまま返して失敗させること。以前はこの関数が必ず成功する形だったため、
// ミドルウェアが一覧を読めなかったリクエストでは除外条件が丸ごと付かず、
// 本来見えないはずの投稿・ユーザーが黙って出ていた（フェイルオープン）。
// エラーを返す形にしてあるのは、新しい一覧クエリを足した人が取りこぼせないようにするため
// （戻り値を無視するとコンパイルが通らない）。
func AppendBlockFilter(ctx context.Context, baseQuery string, args []interface{}, userColumn string) (string, []interface{}, error) {
	// 1. Contextからブロックリストを取得（ミドルウェアがセットしたもの）
	blockedIDs, err := middleware.BlockListFromContext(ctx)
	if err != nil {
		return "", nil, err
	}

	// ブロック対象がいない場合は、元のクエリと引数をそのまま返す
	if len(blockedIDs) == 0 {
		return baseQuery, args, nil
	}

	// 2. プレースホルダー（?, ?, ...）と引数の追加
	placeholders := make([]string, len(blockedIDs))
	for i, id := range blockedIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	// 3. 元のクエリに結合する
	// 例: " AND user_id NOT IN (?, ?)"
	query := fmt.Sprintf("%s AND %s NOT IN (%s)", baseQuery, userColumn, strings.Join(placeholders, ","))

	return query, args, nil
}

// countForPage は「total を数えるかどうか」の分岐を1箇所に閉じ込める。
//
// オフセットページングのリポジトリは、以前は例外なく COUNT と SELECT の2本を
// 撃っていた。GraphQL が total を選んでいないときは COUNT が丸ごと無駄になる
// （しかも WHERE 次第で全件走査になる）ので、repository.PageQuery.WithTotal が
// 立っているときだけ撃つようにした。
//
// 分岐を各リポジトリに書くと、新しい一覧を足した人が素の COUNT を書いてしまい、
// 「この一覧だけ常に数えている」という取りこぼしが静かに増える。一覧系の COUNT は
// 必ずこの関数を通すこと。
//
// WithTotal が false のときの戻りは 0。呼び出し元はそのまま total として返してよい
// （GraphQL 側が total を選んでいないので応答には出ない）。
func countForPage(ctx context.Context, db dbtx, q repository.PageQuery, countSQL string, args ...any) (int, error) {
	if !q.WithTotal {
		return 0, nil
	}
	var total int
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// ■ 可変長プレースホルダの組み立て
//
// IN 句とバルク INSERT の "?" 並べは、これまで各リポジトリが
// strings.Repeat("?,", n) を TrimRight したり TrimSuffix したり、for で
// []string を組んだりと、同じことを3通りで書いていた。書き方が揃っていないと
// 「この一覧だけ1件ずつ撃っている」という取りこぼしが見つけにくいので、
// 以降の IN 句・複数 VALUES は必ず下の2つを通すこと。

// inPlaceholders は IN 句用の "?,?,?" を n 個ぶん返す。n <= 0 なら空文字。
//
// 空文字のまま IN () を組むと MySQL の構文エラーになる。呼び出し側は必ず
// 「引数が空なら DB へ行かずに返す」を先に書くこと（早期 return のほうが、
// 0件のときに無駄な往復もしないので都合が良い）。
func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// valuesPlaceholders は複数 VALUES の INSERT 用に "(?,?),(?,?)" を
// rows 行 × cols 列ぶん返す。どちらかが 0 以下なら空文字。
func valuesPlaceholders(rows, cols int) string {
	if rows <= 0 || cols <= 0 {
		return ""
	}
	row := "(" + inPlaceholders(cols) + ")"
	return strings.TrimSuffix(strings.Repeat(row+",", rows), ",")
}

// maxBulkInsertParams は1本の INSERT に載せるプレースホルダ数の上限。
//
// MySQL のプロトコル上、プリペアドステートメントのパラメータ数は 65535 が上限
// （それを超えると "Prepared statement contains too many placeholders"）。
// 余裕を見て 60000 で切る。1行あたりの列数は呼び出しごとに違うので、
// 「何行まとめてよいか」は bulkInsertChunkSize が列数から出す。
//
// なお上限に当たるのは投票の選択肢や添付のような画面から来る配列ではない
// （UI 側で数十件に収まる）。効いてくるのはページビューのように
// クライアントがまとめて送ってくる配列のほう。
const maxBulkInsertParams = 60000

// bulkInsertChunkSize は cols 列の行を1本の INSERT に何行まで載せてよいかを返す。
// 必ず1以上を返すので、呼び出し側は 0 除算や無限ループを気にしなくてよい。
func bulkInsertChunkSize(cols int) int {
	if cols <= 0 {
		return 1
	}
	n := maxBulkInsertParams / cols
	if n < 1 {
		return 1
	}
	return n
}

// inChunks は rows をプレースホルダ上限に収まる塊に切って exec を呼ぶ。
// cols は1件あたりに使うプレースホルダ数（INSERT なら列数、IN 句なら 1）。
//
// 複数 VALUES の INSERT と IN 句の DELETE は必ずこれを通すこと。上限に届かない
// 箇所では exec が1回だけ呼ばれるので、「分割が要るかどうか」を呼び出し側が
// その都度判断せずに済む（判断を各所に散らすと、分割を忘れた箇所が
// 「件数が増えたときだけ落ちる」という形で残る）。
//
// 実際に分割が起きるのはクライアントが件数を決める配列（セッションのページ
// ビューなど）だけ。投票の選択肢・添付・時間割のように画面の作りで数十件に
// 収まるものは、通しても exec 1回のままで費用は増えない。
func inChunks[T any](rows []T, cols int, exec func(chunk []T) error) error {
	if len(rows) == 0 {
		return nil
	}
	size := bulkInsertChunkSize(cols)
	for start := 0; start < len(rows); start += size {
		end := start + size
		if end > len(rows) {
			end = len(rows)
		}
		if err := exec(rows[start:end]); err != nil {
			return err
		}
	}
	return nil
}
