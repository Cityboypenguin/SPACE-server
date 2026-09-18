package graph

import (
	"context"
	"sort"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	analyticsusecase "github.com/Cityboypenguin/SPACE-server/usecase/analytics"
)

// ■ スキーマと「フィールド名 → 集計」の対応表がずれていないこと
//
// 対応表（infra/mysql の serves と系列名）に無い名前がスキーマに増えると、
// そのフィールドは誰にも養われず「選んだのに常に 0」になる。逆に対応表だけに
// 残った名前は、消えたフィールドのために SQL を走らせ続ける。どちらも
// 応答を見ているだけでは気づきにくいので、名前の集合そのものを突き合わせる。
func TestSummaryFieldNamesMatchSchema(t *testing.T) {
	assertSameFieldNames(t, "AnalyticsSummary", schemaFieldNames(t, "AnalyticsSummary"), mysql.SummaryFieldNames())
}

func TestTimeSeriesFieldNamesMatchSchema(t *testing.T) {
	assertSameFieldNames(t, "TimeSeriesPoint", schemaFieldNames(t, "TimeSeriesPoint"), mysql.TimeSeriesFieldNames())
}

func schemaFieldNames(t *testing.T, typeName string) []string {
	t.Helper()
	def, ok := parsedSchema.Types[typeName]
	if !ok {
		t.Fatalf("type %s is missing from the schema", typeName)
	}
	names := make([]string, 0, len(def.Fields))
	for _, f := range def.Fields {
		if f.Name == "__typename" {
			continue
		}
		names = append(names, f.Name)
	}
	return names
}

func assertSameFieldNames(t *testing.T, typeName string, schema, covered []string) {
	t.Helper()
	inSchema := map[string]bool{}
	for _, n := range schema {
		inSchema[n] = true
	}
	inTable := map[string]bool{}
	for _, n := range covered {
		inTable[n] = true
	}

	var missing, extra []string
	for n := range inSchema {
		if !inTable[n] {
			missing = append(missing, n)
		}
	}
	for n := range inTable {
		if !inSchema[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("%s: スキーマにあるが対応表に無い（選んでも常に 0 になる）: %v", typeName, missing)
	}
	if len(extra) > 0 {
		t.Errorf("%s: 対応表にあるがスキーマに無い（消えたフィールドのために集計している）: %v", typeName, extra)
	}
}

// ■ 選択されたフィールドだけが集計へ運ばれること
//
// 実行器を通さないと選択集合が無く、判定は「全部計算する」へ倒れる。
// リゾルバを直接呼ぶテストでは確かめられないので、本物のクエリを投げる。

type recordingAnalyticsUseCase struct {
	got repository.FieldSet
}

func (u *recordingAnalyticsUseCase) Execute(ctx context.Context, fields repository.FieldSet) (*model.AnalyticsSummary, error) {
	u.got = fields
	return &model.AnalyticsSummary{}, nil
}

type recordingTimeSeriesUseCase struct {
	got repository.FieldSet
}

func (u *recordingTimeSeriesUseCase) Execute(ctx context.Context, granularity, from, to string, series repository.FieldSet) ([]*model.TimeSeriesPoint, error) {
	u.got = series
	return nil, nil
}

var (
	_ analyticsusecase.GetAnalyticsUseCase  = (*recordingAnalyticsUseCase)(nil)
	_ analyticsusecase.GetTimeSeriesUseCase = (*recordingTimeSeriesUseCase)(nil)
)

func TestAdminGetAnalyticsPassesOnlySelectedFields(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "選んだ2つだけ運ぶ",
			query: `{ adminGetAnalytics { totalUsers totalPosts } }`,
			want:  []string{"totalUsers", "totalPosts"},
		},
		{
			name:  "別名でもスキーマ上の名前で運ぶ",
			query: `{ adminGetAnalytics { users: totalUsers } }`,
			want:  []string{"totalUsers"},
		},
		{
			name:  "フラグメントの中も拾う",
			query: `{ adminGetAnalytics { ...f } } fragment f on AnalyticsSummary { dau mau }`,
			want:  []string{"dau", "mau"},
		},
		{
			name:  "@skip(if: true) は応答に出ないので運ばない",
			query: `{ adminGetAnalytics { totalUsers totalPosts @skip(if: true) } }`,
			want:  []string{"totalUsers"},
		},
		{
			name:  "派生値も名前のまま運ぶ（どの集計が要るかはリポジトリが決める）",
			query: `{ adminGetAnalytics { avgLikesPerPost } }`,
			want:  []string{"avgLikesPerPost"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &recordingAnalyticsUseCase{}
			c := newSelectionTestClient(t, &Resolver{GetAnalyticsUseCase: uc}, adminClaims(1))
			var resp map[string]any
			if err := c.Post(tc.query, &resp); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			assertFieldSet(t, uc.got, tc.want)
		})
	}
}

func TestAdminGetTimeSeriesPassesOnlySelectedSeries(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "選んだ系列だけ運ぶ",
			query: `{ adminGetTimeSeries(granularity: day, from: "2026-01-01", to: "2026-01-31") { points { label posts } } }`,
			want:  []string{"label", "posts"},
		},
		{
			name:  "activeUsers を選べば運ぶ",
			query: `{ adminGetTimeSeries(granularity: hour, from: "2026-01-01", to: "2026-01-02") { points { activeUsers } } }`,
			want:  []string{"activeUsers"},
		},
		{
			name:  "points を選ばなければ系列は空",
			query: `{ adminGetTimeSeries(granularity: day, from: "2026-01-01", to: "2026-01-31") { __typename } }`,
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &recordingTimeSeriesUseCase{}
			c := newSelectionTestClient(t, &Resolver{GetTimeSeriesUseCase: uc}, adminClaims(1))
			var resp map[string]any
			if err := c.Post(tc.query, &resp); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			assertFieldSet(t, uc.got, tc.want)
		})
	}
}

// assertFieldSet は「want のものだけが要求されている」ことを確かめる。
// 余計なものが混ざっていないかも見たいので、スキーマの全フィールドを走査する。
func assertFieldSet(t *testing.T, got repository.FieldSet, want []string) {
	t.Helper()
	wanted := map[string]bool{}
	for _, n := range want {
		wanted[n] = true
		if !got.Wants(n) {
			t.Errorf("%q が要求として運ばれていない", n)
		}
	}
	for _, n := range append(mysql.SummaryFieldNames(), mysql.TimeSeriesFieldNames()...) {
		if !wanted[n] && got.Wants(n) {
			t.Errorf("%q は選ばれていないのに要求として運ばれている", n)
		}
	}
}
