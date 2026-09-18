package metrics

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// ResponseTimePercentiles は Percentile を3回呼ぶのと完全に同じ値を返さなければ
// ならない（管理画面に出る数字なので、ソートを1回に畳んだ副作用で値が動いては
// 困る）。リングバッファの状態が変わるところ（0件・1件・未充填・ちょうど満杯・
// 一周して上書き済み）を境界として並べてある。
func TestResponseTimePercentilesMatchPercentile(t *testing.T) {
	sizes := []int{0, 1, 2, 3, 99, ringBufferSize - 1, ringBufferSize, ringBufferSize + 7}

	for _, n := range sizes {
		n := n
		t.Run(name(n), func(t *testing.T) {
			m := &Metrics{}
			rng := rand.New(rand.NewSource(int64(n) + 1))
			for i := 0; i < n; i++ {
				m.RecordRequest(rng.Float64()*1000, false)
			}

			got := m.ResponseTimePercentiles()
			want := Percentiles{
				P50: m.Percentile(50),
				P95: m.Percentile(95),
				P99: m.Percentile(99),
			}
			if got != want {
				t.Fatalf("with %d samples: ResponseTimePercentiles() = %+v, want %+v", n, got, want)
			}
		})
	}
}

// 0件のときは3値とも 0（ゼロ除算やパニックではなく 0 を返す、が従来の挙動）。
func TestResponseTimePercentilesEmpty(t *testing.T) {
	m := &Metrics{}
	if got := (m.ResponseTimePercentiles()); got != (Percentiles{}) {
		t.Fatalf("ResponseTimePercentiles() with no samples = %+v, want all zero", got)
	}
}

// 1件だけなら3値ともその1件。
func TestResponseTimePercentilesSingleSample(t *testing.T) {
	m := &Metrics{}
	m.RecordRequest(42, false)
	want := Percentiles{P50: 42, P95: 42, P99: 42}
	if got := m.ResponseTimePercentiles(); got != want {
		t.Fatalf("ResponseTimePercentiles() with one sample = %+v, want %+v", got, want)
	}
}

// 既知の値で添字の丸めごと固定する。1..100 を入れると、従来の
// idx = int(p/100*(size-1) + 0.5) は p50 -> data[50] = 51 になる。
func TestResponseTimePercentilesKnownValues(t *testing.T) {
	m := &Metrics{}
	for i := 1; i <= 100; i++ {
		m.RecordRequest(float64(i), false)
	}
	want := Percentiles{P50: 51, P95: 95, P99: 99}
	if got := m.ResponseTimePercentiles(); got != want {
		t.Fatalf("ResponseTimePercentiles() = %+v, want %+v", got, want)
	}
}

// ApplyToSummary が読むのは Global なので、そこまで通しで確かめる。
func TestApplyToSummaryFillsPercentiles(t *testing.T) {
	saved := Global
	t.Cleanup(func() { Global = saved })

	Global = &Metrics{}
	for i := 1; i <= 100; i++ {
		Global.RecordRequest(float64(i), i > 90) // 10% をエラーにする
	}
	Global.IncWSConnections()
	Global.IncSSEConnections()
	Global.IncSSEConnections()

	var s model.AnalyticsSummary
	ApplyToSummary(&s)

	if s.P50ResponseTimeMs != 51 || s.P95ResponseTimeMs != 95 || s.P99ResponseTimeMs != 99 {
		t.Fatalf("percentiles = %v/%v/%v, want 51/95/99",
			s.P50ResponseTimeMs, s.P95ResponseTimeMs, s.P99ResponseTimeMs)
	}
	if s.WebSocketConnections != 1 || s.SSEConnections != 2 {
		t.Fatalf("connections = ws %d / sse %d, want 1 / 2", s.WebSocketConnections, s.SSEConnections)
	}
	if s.ErrorRate5xx != 10 {
		t.Fatalf("error rate = %v, want 10", s.ErrorRate5xx)
	}
}

func name(n int) string {
	switch n {
	case 0:
		return "empty"
	case ringBufferSize:
		return "exactly_full"
	default:
		return "n_" + strconv.Itoa(n)
	}
}
