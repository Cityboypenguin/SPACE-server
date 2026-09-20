package graph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	gqlast "github.com/vektah/gqlparser/v2/ast"
)

// ■ fieldRequested に書いた文字列パスが、本当にスキーマ上に在ること
//
// 元の指摘はこうだった: fieldRequested(ctx, "items", "memberCount") のような
// 選択判定はスキーマのフィールド名を文字列で持つので、
//
//   - スキーマ側で memberCount を別名に変えても Go はコンパイルが通る。判定は
//     恒久的に false へ倒れ、「選んだのに 0 が返る」という、応答を見ても
//     "そういう値なのだろう" としか思えない壊れ方をする。
//   - 最初から綴りを間違えても同じ。誰も気づけない。
//
// 文字列を定数にしても「実在しない名前を書ける」は解けない（定数の中身が
// 間違っていられる）ので、Analytics の対応表テスト（analytics_selection_test.go）
// と同じ考え方で、**スキーマそのものと突き合わせる**。
//
// やり方: graph パッケージの Go ソースを構文解析して fieldRequested の呼び出しを
// 全部集め、呼び出し元リゾルバの戻り値の型（= いま解決中のフィールドの型）を
// 起点にパスをスキーマ上で辿る。辿れなければ落とす。
//
// 参照するスキーマは生成済みの parsedSchema（generated.go）。
// analytics_selection_test.go / user_email_boundary_test.go と同じ出所に揃えてある。

// fieldRequestedHelpers は「リゾルバではないのに fieldRequested を呼ぶ」共通ヘルパー。
//
// これらは解決中のフィールドの型が呼び出し元によって変わるので、上の
// 「戻り値の型を起点に辿る」やり方では検証できない。代わりに、この下の
// 専用テストがそれぞれの名前をスキーマと突き合わせている。
// 新しくヘルパーを足したら、ここへ理由と一緒に足し、突き合わせも必ず書くこと。
var fieldRequestedHelpers = map[string]string{
	"roomReadStatusRequested": "パスは roomReadStatusFields から組む。TestRoomReadStatusFieldsExistOnRoom が Room と突き合わせる",
	"resolvePageQuery":        "\"total\" を全ページ型に対して聞く。TestEveryPageTypeHasTotalField が全 *Page 型と突き合わせる",
}

type fieldRequestedCall struct {
	file     string
	line     int
	funcName string
	rootType string // 起点になる GraphQL の型名
	path     []string
}

func TestFieldRequestedPathsExistInSchema(t *testing.T) {
	calls, unresolved := collectFieldRequestedCalls(t)

	for _, u := range unresolved {
		t.Errorf("%s: %s は fieldRequested を呼んでいるが、起点になる GraphQL の型が分からない。\n"+
			"  リゾルバなら *gqlmodel.<型> を返しているか確かめること。\n"+
			"  共通ヘルパーなら fieldRequestedHelpers へ理由付きで足し、名前をスキーマと突き合わせるテストも書くこと。",
			u.pos, u.funcName)
	}

	if len(calls) == 0 {
		t.Fatal("fieldRequested の呼び出しが1つも見つからなかった。解析が壊れている可能性が高い")
	}

	for _, c := range calls {
		def, ok := parsedSchema.Types[c.rootType]
		if !ok {
			t.Errorf("%s:%d %s: 戻り値の型 %s がスキーマに無い", c.file, c.line, c.funcName, c.rootType)
			continue
		}
		if err := walkSchemaPath(def, c.path); err != "" {
			t.Errorf("%s:%d %s: fieldRequested(ctx, %s) のパスがスキーマ上に無い（%s から辿れない）: %s\n"+
				"  スキーマ側の名前を変えたなら、この呼び出しも直すこと。直さないと判定は常に false になり、"+
				"選んだのに値が返らなくなる。",
				c.file, c.line, c.funcName, quotedPath(c.path), c.rootType, err)
		}
	}
}

// walkSchemaPath は path をスキーマ上で辿る。辿れない場合は理由を返す。
// 名前付き型の皮（[T!]! など）は ast.Type.Name() が剥いでくれる。
func walkSchemaPath(def *gqlast.Definition, path []string) string {
	cur := def
	for i, name := range path {
		f := cur.Fields.ForName(name)
		if f == nil {
			return cur.Name + " に " + name + " というフィールドが無い"
		}
		if i == len(path)-1 {
			return ""
		}
		next, ok := parsedSchema.Types[f.Type.Name()]
		if !ok {
			return cur.Name + "." + name + " の型 " + f.Type.Name() + " がスキーマに無い"
		}
		cur = next
	}
	return ""
}

func quotedPath(path []string) string {
	parts := make([]string, 0, len(path))
	for _, p := range path {
		parts = append(parts, strconv.Quote(p))
	}
	return strings.Join(parts, ", ")
}

type unresolvedCaller struct {
	pos      string
	funcName string
}

// collectFieldRequestedCalls は graph パッケージの Go ソースから fieldRequested の
// 呼び出しを集める。起点の型は「呼び出し元の関数の第1戻り値」＝ gqlgen が生成した
// リゾルバの戻り値＝いま解決中のフィールドの型、から取る。
func collectFieldRequestedCalls(t *testing.T) ([]fieldRequestedCall, []unresolvedCaller) {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("ソースの列挙に失敗: %v", err)
	}

	fset := token.NewFileSet()
	var calls []fieldRequestedCall
	var unresolved []unresolvedCaller
	usedHelpers := map[string]bool{}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("%s の構文解析に失敗: %v", file, err)
		}

		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			found := fieldRequestedCallsIn(fn)
			if len(found) == 0 {
				continue
			}
			if _, ok := fieldRequestedHelpers[fn.Name.Name]; ok {
				usedHelpers[fn.Name.Name] = true
				continue
			}
			rootType, ok := graphTypeOfResult(fn)
			if !ok {
				unresolved = append(unresolved, unresolvedCaller{
					pos:      fset.Position(fn.Pos()).String(),
					funcName: fn.Name.Name,
				})
				continue
			}
			for _, c := range found {
				pos := fset.Position(c.pos)
				if !c.allLiterals {
					// リゾルバの中でパスを組み立てているなら、そのぶん検証できない。
					// ヘルパーへ切り出して fieldRequestedHelpers に載せること。
					unresolved = append(unresolved, unresolvedCaller{pos: pos.String(), funcName: fn.Name.Name})
					continue
				}
				calls = append(calls, fieldRequestedCall{
					file:     filepath.Base(file),
					line:     pos.Line,
					funcName: fn.Name.Name,
					rootType: rootType,
					path:     c.path,
				})
			}
		}
	}

	// 許可リストが現実と乖離していないか（消えたヘルパーが残っていないか）も見る。
	var stale []string
	for name := range fieldRequestedHelpers {
		if !usedHelpers[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("fieldRequestedHelpers に実在しない（もう fieldRequested を呼んでいない）ヘルパーが残っている: %s", name)
	}

	return calls, unresolved
}

type rawCall struct {
	pos         token.Pos
	path        []string
	allLiterals bool
}

func fieldRequestedCallsIn(fn *ast.FuncDecl) []rawCall {
	var out []rawCall
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "fieldRequested" || len(call.Args) == 0 {
			return true
		}
		rc := rawCall{pos: call.Pos(), allLiterals: true}
		for _, arg := range call.Args[1:] { // 先頭は ctx
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				rc.allLiterals = false
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				rc.allLiterals = false
				continue
			}
			rc.path = append(rc.path, s)
		}
		out = append(out, rc)
		return true
	})
	return out
}

// graphTypeOfResult は関数の第1戻り値から GraphQL の型名を取る。
// gqlgen のリゾルバは *gqlmodel.T か []*gqlmodel.T を返し、gqlgen.yml に
// autobind もモデル差し替えも無いので、Go の型名＝スキーマの型名でよい。
func graphTypeOfResult(fn *ast.FuncDecl) (string, bool) {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return "", false
	}
	expr := fn.Type.Results.List[0].Type
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.ArrayType:
			expr = t.Elt
		case *ast.SelectorExpr:
			pkg, ok := t.X.(*ast.Ident)
			if !ok || pkg.Name != "gqlmodel" {
				return "", false
			}
			return t.Sel.Name, true
		default:
			return "", false
		}
	}
}

// roomReadStatusFields は Room の既読まわりを1回の取得で埋めるための名前の束。
// 判定は fieldRequested だがパスを組み立てて渡すので、名前だけここで突き合わせる。
func TestRoomReadStatusFieldsExistOnRoom(t *testing.T) {
	def, ok := parsedSchema.Types["Room"]
	if !ok {
		t.Fatal("Room がスキーマに無い")
	}
	for _, name := range roomReadStatusFields {
		if def.Fields.ForName(name) == nil {
			t.Errorf("roomReadStatusFields の %q が Room に無い。スキーマ側で名前を変えたなら合わせること", name)
		}
	}
}

// resolvePageQuery は「ページ型の直下に total がある」ことに乗っている。
// total を持たないページ型を作ると、その一覧だけ COUNT が走らなくなる（total は
// そもそも無いので応答は変わらないが、次に total を足した人が「選んだのに 0」を踏む）。
//
// 総件数を持たないページ型（カーソルページング）はここに理由付きで並べる。
var pageTypesWithoutTotal = map[string]string{
	"MessagePage": "カーソルページング。総件数ではなく hasMoreBefore / hasMoreAfter で続きの有無を示す",
}

func TestEveryPageTypeHasTotalField(t *testing.T) {
	seen := map[string]bool{}
	for name, def := range parsedSchema.Types {
		if def.Kind != gqlast.Object || !strings.HasSuffix(name, "Page") {
			continue
		}
		seen[name] = true
		if _, ok := pageTypesWithoutTotal[name]; ok {
			continue
		}
		if def.Fields.ForName("total") == nil {
			t.Errorf("%s は *Page 型なのに total を持たない。resolvePageQuery の判定が効かない", name)
		}
	}
	// 例外リストが現実と乖離していないか（消えた型・total を持つようになった型が
	// 残っていないか）も見る。残っていると検査から静かに外れ続ける。
	for name := range pageTypesWithoutTotal {
		def, ok := parsedSchema.Types[name]
		if !ok || !seen[name] {
			t.Errorf("pageTypesWithoutTotal に実在しないページ型が残っている: %s", name)
			continue
		}
		if def.Fields.ForName("total") != nil {
			t.Errorf("%s は total を持つようになった。pageTypesWithoutTotal から外すこと", name)
		}
	}
}
