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

// limitArg はフィールドの引数から「実際に返りうる最大件数」を取り出す。
//
// gqlgen は既定値を適用したあとの引数を渡すので、クエリが limit を省いていても
// スキーマの既定値（posts なら 20、replies なら 50）がここへ来る。
//
// 要求された値をそのまま使わず、サーバーが実際に返す件数へ寄せるのが要。
// リゾルバ側は limit を maxPageSize / maxMessagePageSize で頭打ちにするので
// （graph/helpers.go の resolveLimit）、要求値のままだと2つずれる。
//
//   - 上振れ: limit: 1000000 は 200 件しか返らないのに、100万件ぶんの重さと
//     数えられて拒否される。丸められるはずのクエリが「大きすぎます」で落ちる。
//   - 下振れ: limit を省ける後付けのコレクション（Post.favorites）は、
//     引数が無いと重みが付かないのに、実際は unpagedCollectionCap 件まで返る。
//
// どちらもクエリの複雑度が実際の仕事量を表していない状態なので、ここで揃える。
//
// 型が int32 なのはスキーマの Int に対応する生成型がそうだから。将来 int へ
// 変わっても気づけるよう、両方受ける。
func limitArg(args map[string]any) (int, bool) {
	raw, ok := args["limit"]
	if !ok {
		// limit 引数そのものが無いフィールド。件数で重み付けする対象ではない。
		return 0, false
	}

	switch v := raw.(type) {
	case int32:
		return effectiveLimit(int(v)), true
	case *int32:
		if v == nil {
			return unpagedCollectionCap, true
		}
		return effectiveLimit(int(*v)), true
	case int:
		return effectiveLimit(v), true
	case *int:
		if v == nil {
			return unpagedCollectionCap, true
		}
		return effectiveLimit(*v), true
	case int64:
		return effectiveLimit(int(v)), true
	}
	return 0, false
}

// effectiveLimit は要求された件数を、サーバーが実際に返しうる上限へ寄せる。
//
// 上限に maxMessagePageSize（一番緩いフィールドの上限）を使うのは、複雑度からは
// どのフィールドかが分からないため。フィールドごとの上限より緩い側へ倒しておけば、
// 実際に返る件数より軽く数えてしまうことは無い。
func effectiveLimit(requested int) int {
	if requested < 0 {
		return 0
	}
	if requested > maxMessagePageSize {
		return maxMessagePageSize
	}
	return requested
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
	// 省略できる limit を持つフィールドは、省かれても件数ぶん数える。
	//
	// gqlgen は「既定値が無く、送られてもいない引数」を args に入れてこない。
	// つまり args を見るだけでは「limit を取らないフィールド」と「limit を
	// 省かれたフィールド」の区別が付かない。区別しないと、引数を送らないだけで
	// 重み付けを回避できてしまう（Post.favorites は省かれても
	// unpagedCollectionCap 件まで返る）。スキーマ側の定義で見分ける。
	if w.declaresLimitArgument(typeName, fieldName) {
		return listComplexity(childComplexity, unpagedCollectionCap), true
	}
	return 0, false
}

// declaresLimitArgument は typeName.fieldName が limit 引数を持つかを返す。
func (w weightedComplexitySchema) declaresLimitArgument(typeName, fieldName string) bool {
	schema := w.ExecutableSchema.Schema()
	if schema == nil {
		return false
	}
	def, ok := schema.Types[typeName]
	if !ok || def == nil {
		return false
	}
	field := def.Fields.ForName(fieldName)
	if field == nil {
		return false
	}
	return field.Arguments.ForName("limit") != nil
}

// NewWeightedExecutableSchema は本番とテストが同じ数え方を使うための唯一の入口。
// 数え方を変えるときはここを変えれば、複雑度のテストもそのまま追従する。
func NewWeightedExecutableSchema(cfg Config) graphql.ExecutableSchema {
	return weightedComplexitySchema{NewExecutableSchema(cfg)}
}

// ComplexityLimit はサーバーが受け付けるクエリの複雑度の上限。
//
// 決め方は2段。
//
//  1. いまクライアントが実際に投げているクエリの最大は、チャット履歴の 2200。
//  2. クライアントを変えずともサーバーが許してしまう最大は、同じチャット履歴を
//     maxMessagePageSize（200）いっぱいで引いた 8800。上限はクライアントの
//     いまの都合ではなく、サーバーが許す範囲で決める必要がある。
//
// この値は 2 の上に置いてある。1 に対しては5倍以上の余裕があるので、
// 画面にフィールドを足したくらいで本番が落ちることはない。
//
// 一方、防ぎたい入れ子の展開は桁で外れる。返信を2階層 100 件ずつ開くと
// 3万を超えるので、実クエリを通すためにここまで上げてもなお届かない。
//
// 上げるときは「どの実クエリが通らなかったか」を、下げるときは「何を弾きたいか」を
// complexity_test.go に足してから動かすこと。数字だけ動かすと、次に触る人には
// なぜその値なのかが分からない。
const ComplexityLimit = 12000
