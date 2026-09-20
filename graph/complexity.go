package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
)

// GraphQL の複雑度の重み付け。
//
// 固定上限（FixedComplexityLimit）だけだと、クエリの「大きさ」が選んだフィールドの
// 数でしか測られない。実際に走る仕事は返す件数に比例するので、件数を引数で指定できる
// フィールドでは両者が大きくずれる。
//
//	query {                       # フィールド数は十数個。件数を見ない上限は楽に通る。
//	  getPostByID(id: "1") {
//	    replies(limit: 100) {     # 100件
//	      replies(limit: 100) {   # × 100件
//	        replies(limit: 100) { # × 100件 = 100万件
//	          ID content
//	        }
//	      }
//	    }
//	  }
//	}
//
// 掛け算は入れ子の一覧どうしだけでなく、一覧の中に一覧を置いても起きる。
//
//	questions(roomID: "1", limit: 50) {  # 50件
//	  items { answers(limit: 100) {      # × 100件 = 5000件
//	    items { ID body }
//	  } }
//	}
//
// そこで limit を取るフィールドは、その子の複雑度に返却件数を掛ける。入れ子の
// 段数ぶん掛け合わさるので、上の2つはどちらも上限を大きく超えて弾かれる。
//
// ComplexityRoot にフィールドを1つずつ書き並べていないのは、書き漏らしを
// 無くすため。一覧は50個近くあり、あとからスキーマに足されたものは
// 「重みを付け忘れた1つ」として静かに素通しになる。ここでは生成コードの
// Complexity を包んで、limit を持つフィールドすべてに機械的に効かせている。

// listComplexity は「(子の複雑度 + 1) × 返却件数」。
//
// 子の複雑度に1を足すのは、フィールドを1つしか選んでいない一覧
// （`replies(limit: 100) { ID }` など）でも件数が効くようにするため。
func listComplexity(childComplexity int, n int) int {
	if n < 0 {
		n = 0
	}
	return (childComplexity + 1) * n
}

// limitArg はフィールドの引数から返却件数を取り出す。
//
// gqlgen は既定値を適用したあとの引数を渡すので、クエリが limit を省いていても
// スキーマの既定値（posts なら 20、replies なら 50）がここへ来る。
//
// 型が int32 なのはスキーマの Int に対応する生成型がそうだから。将来 int へ
// 変わっても気づけるよう、両方受ける。
func limitArg(args map[string]any) (int, bool) {
	raw, ok := args["limit"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int32:
		return int(v), true
	case *int32:
		if v == nil {
			return 0, false
		}
		return int(*v), true
	case int:
		return v, true
	case *int:
		if v == nil {
			return 0, false
		}
		return *v, true
	case int64:
		return int(v), true
	}
	return 0, false
}

// weightedComplexitySchema は生成された ExecutableSchema に、件数による
// 重み付けだけを足して包んだもの。
//
// 包む先の Complexity が答えを返した場合はそちらを優先する（ComplexityRoot に
// 手で重みを書いた場合に、こちらが上書きしてしまわないように）。
type weightedComplexitySchema struct {
	graphql.ExecutableSchema
}

func (w weightedComplexitySchema) Complexity(
	ctx context.Context,
	typeName, fieldName string,
	childComplexity int,
	args map[string]any,
) (int, bool) {
	if custom, ok := w.ExecutableSchema.Complexity(ctx, typeName, fieldName, childComplexity, args); ok {
		return custom, true
	}
	if n, ok := limitArg(args); ok {
		return listComplexity(childComplexity, n), true
	}
	return 0, false
}

// NewWeightedExecutableSchema は本番とテストが同じ数え方を使うための唯一の入口。
// 数え方を変えるときはここを変えれば、複雑度のテストもそのまま追従する。
func NewWeightedExecutableSchema(cfg Config) graphql.ExecutableSchema {
	return weightedComplexitySchema{NewExecutableSchema(cfg)}
}

// ComplexityLimit はサーバーが受け付けるクエリの複雑度の上限。
//
// 決め方: クライアントが実際に投げている全クエリに、呼び出し側が渡しうる
// 一番大きい件数（200。管理画面のメッセージ一覧と質問一覧がこれを渡す）を
// 当てはめて測り、その最大値がチャット履歴の 8800 だった。そこに 1.7 倍ほどの
// 余裕を見てこの値にしてある。
//
// 一方、防ぎたい入れ子の展開は桁で外れる。返信を2階層 100 件ずつ開くと
// 3万を超えるので、上限を実クエリに合わせて上げてもなお届かない。
//
// 上げるときは「どの実クエリが通らなかったか」を、下げるときは「何を弾きたいか」を
// complexity_test.go に足してから動かすこと。数字だけ動かすと、次に触る人には
// なぜその値なのかが分からない。
const ComplexityLimit = 15000
