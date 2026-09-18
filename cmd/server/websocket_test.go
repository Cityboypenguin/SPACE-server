package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	coderws "github.com/coder/websocket"

	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/connlimit"
	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
)

// ■ このファイルが守っているもの
//
// gqlgen v0.17.95 で transport.Websocket.Upgrader (gorilla/websocket) が廃止され、
// WebSocket 実装が coder/websocket に差し替わった。オリジン検証は Upgrader.CheckOrigin
// ではなく WebsocketImplementation の差し替えで行うようになったため、
//
//   - 許可オリジンは今までどおり接続できること
//   - 許可していないオリジンは今までどおり拒否されること（Origin ヘッダー無しを含む）
//   - ハンドシェイクを抜けた先で subscription が実際に配信されること
//
// を、実物の HTTP サーバ + 実物の WebSocket クライアントで確かめる。
// coder/websocket の既定は「Origin のホストがリクエストホストと一致すれば許可、
// Origin 無しも許可」で許可リスト方式と意味が違うので、既定に戻っていないことの
// 回帰テストでもある。

func TestIsOriginAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{"https://app.example.com", "http://localhost:3000"}

	tests := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"許可リストにあるオリジン", "https://app.example.com", allowed, true},
		{"許可リストにある2つ目のオリジン", "http://localhost:3000", allowed, true},
		{"許可リストに無いオリジン", "https://evil.example.com", allowed, false},
		{"スキームだけ違うオリジン", "http://app.example.com", allowed, false},
		{"ポートだけ違うオリジン", "http://localhost:3001", allowed, false},
		{"Origin が空", "", allowed, false},
		{"ワイルドカードは全部許可", "https://evil.example.com", []string{"*"}, true},
		{"ワイルドカードは Origin 空も許可", "", []string{"*"}, true},
		{"許可リストが空なら全部拒否", "https://app.example.com", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isOriginAllowed(tt.origin, tt.allowed); got != tt.want {
				t.Errorf("isOriginAllowed(%q, %v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

// newTestGraphQLServer は main.go と同じ WebSocket 設定（実装の差し替えによる
// オリジン検証、KeepAlive、InitFunc/CloseFunc）を持つサーバを返す。
// InitFunc はトークン検証の代わりに固定の管理者クレームを載せる（ここで確かめたいのは
// ハンドシェイクと配信であって JWT ではない）。
func newTestGraphQLServer(t *testing.T, allowedOrigins []string, ps *pubsub.PubSub) *httptest.Server {
	t.Helper()

	resolver := &graph.Resolver{PubSub: ps}
	gqlServer := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	gqlServer.AddTransport(transport.Websocket{
		Implementation:        newOriginCheckingWebsocketImplementation(allowedOrigins),
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc: func(ctx context.Context, _ transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			return auth.WithClaims(ctx, &auth.Claims{ID: 1, Role: "admin"}), nil, nil
		},
	})
	gqlServer.AddTransport(transport.POST{})

	srv := httptest.NewServer(gqlServer)
	t.Cleanup(srv.Close)
	return srv
}

// dialWS は origin を名乗って WebSocket ハンドシェイクを試みる。
func dialWS(t *testing.T, srv *httptest.Server, origin string) (*coderws.Conn, *http.Response, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}

	return coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &coderws.DialOptions{
		HTTPHeader:   header,
		Subprotocols: []string{"graphql-transport-ws"},
		// Origin ヘッダーを送らないケースを作るため、クライアント側の自動付与は使わない。
		HTTPClient: srv.Client(),
	})
}

func TestWebsocketOriginIsVerified(t *testing.T) {
	allowedOrigins := []string{"https://app.example.com", "http://localhost:3000"}

	t.Run("許可オリジンは接続できる", func(t *testing.T) {
		srv := newTestGraphQLServer(t, allowedOrigins, pubsub.New())

		conn, _, err := dialWS(t, srv, "https://app.example.com")
		if err != nil {
			t.Fatalf("許可したオリジンからの接続が拒否された: %v", err)
		}
		defer conn.CloseNow()

		if got := conn.Subprotocol(); got != "graphql-transport-ws" {
			t.Fatalf("subprotocol = %q, want graphql-transport-ws", got)
		}
	})

	t.Run("許可していないオリジンは拒否される", func(t *testing.T) {
		srv := newTestGraphQLServer(t, allowedOrigins, pubsub.New())

		conn, resp, err := dialWS(t, srv, "https://evil.example.com")
		if err == nil {
			conn.CloseNow()
			t.Fatal("許可していないオリジンからの接続が確立してしまった")
		}
		if resp != nil && resp.StatusCode == http.StatusSwitchingProtocols {
			t.Fatalf("101 を返している（アップグレードが成立している）")
		}
	})

	t.Run("Origin ヘッダー無しは拒否される", func(t *testing.T) {
		// coder/websocket の既定（InsecureSkipVerify=false）は Origin 無しを許可するので、
		// 既定に戻っていたらここで落ちる。
		srv := newTestGraphQLServer(t, allowedOrigins, pubsub.New())

		conn, _, err := dialWS(t, srv, "")
		if err == nil {
			conn.CloseNow()
			t.Fatal("Origin ヘッダー無しの接続が確立してしまった")
		}
	})

	t.Run("同一ホストでも許可リストに無ければ拒否される", func(t *testing.T) {
		// coder/websocket の既定は「リクエストホストと同じ Origin は常に許可」。
		// 許可リストに載っていない以上、それも拒否されなければならない。
		srv := newTestGraphQLServer(t, allowedOrigins, pubsub.New())

		conn, _, err := dialWS(t, srv, srv.URL)
		if err == nil {
			conn.CloseNow()
			t.Fatalf("許可リストに無い同一ホストのオリジン %q で接続が確立してしまった", srv.URL)
		}
	})

	t.Run("ワイルドカードは任意のオリジンを許可する", func(t *testing.T) {
		srv := newTestGraphQLServer(t, []string{"*"}, pubsub.New())

		conn, _, err := dialWS(t, srv, "https://anything.example.com")
		if err != nil {
			t.Fatalf("ワイルドカード設定で接続が拒否された: %v", err)
		}
		conn.CloseNow()
	})
}

// wsMessage は graphql-transport-ws プロトコルのメッセージ。
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func writeWS(t *testing.T, ctx context.Context, conn *coderws.Conn, msg wsMessage) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal a websocket message: %v", err)
	}
	if err := conn.Write(ctx, coderws.MessageText, data); err != nil {
		t.Fatalf("failed to write a websocket message: %v", err)
	}
}

func readWS(t *testing.T, ctx context.Context, conn *coderws.Conn) wsMessage {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read a websocket message: %v", err)
	}
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("failed to unmarshal a websocket message %s: %v", data, err)
	}
	return msg
}

// 許可オリジンで張った接続の上で、subscription が実際にイベントを受け取れること。
// ハンドシェイクだけ通って配信が死んでいる、という壊れ方を拾う。
func TestWebsocketSubscriptionDelivery(t *testing.T) {
	ps := pubsub.New()
	srv := newTestGraphQLServer(t, []string{"https://app.example.com"}, ps)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := dialWS(t, srv, "https://app.example.com")
	if err != nil {
		t.Fatalf("接続できなかった: %v", err)
	}
	defer conn.CloseNow()

	writeWS(t, ctx, conn, wsMessage{Type: "connection_init", Payload: json.RawMessage(`{}`)})
	if ack := readWS(t, ctx, conn); ack.Type != "connection_ack" {
		t.Fatalf("connection_ack が返らなかった: %+v", ack)
	}

	writeWS(t, ctx, conn, wsMessage{
		ID:      "sub-1",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { adminCourseImportStatusUpdated { state processedCount totalCount } }"}`),
	})

	// gqlgen は subscribe を受けてから購読を登録するので、登録前に publish すると
	// 誰にも届かない。読み取りは別ゴルーチンに寄せ（coder/websocket は read に渡した
	// context が切れると接続ごと閉じるため、短い context で読み直すことはできない）、
	// 届くまで publish を繰り返す。
	received := make(chan wsMessage, 1)
	readErr := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				readErr <- err
				return
			}
			// KeepAlive の ping/pong など配信以外のメッセージは読み飛ばす。
			if msg.Type == "next" || msg.Type == "error" {
				received <- msg
				return
			}
		}
	}()

	status := courseimport.Status{
		State:     courseimport.StateRunning,
		Processed: 3,
		Total:     10,
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(5 * time.Second)

	var msg wsMessage
	for msg.Type == "" {
		select {
		case msg = <-received:
		case err := <-readErr:
			t.Fatalf("WebSocket の読み取りが失敗した: %v", err)
		case <-ticker.C:
			ps.Publish(graph.CourseImportStatusTopic, status)
		case <-timeout:
			t.Fatal("subscription のイベントが届かなかった")
		}
	}

	if msg.Type == "error" {
		t.Fatalf("subscription がエラーを返した: %s", msg.Payload)
	}

	var payload struct {
		Data struct {
			AdminCourseImportStatusUpdated struct {
				State          string `json:"state"`
				ProcessedCount *int   `json:"processedCount"`
				TotalCount     *int   `json:"totalCount"`
			} `json:"adminCourseImportStatusUpdated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal the payload %s: %v", msg.Payload, err)
	}
	got := payload.Data.AdminCourseImportStatusUpdated
	if got.State != "RUNNING" {
		t.Fatalf("state = %q, want RUNNING", got.State)
	}
	if got.ProcessedCount == nil || *got.ProcessedCount != 3 {
		t.Fatalf("processedCount = %v, want 3", got.ProcessedCount)
	}
	if got.TotalCount == nil || *got.TotalCount != 10 {
		t.Fatalf("totalCount = %v, want 10", got.TotalCount)
	}
}

// InitFunc で確保した接続枠が CloseFunc で必ず返ること。
// 実装差し替えで CloseFunc が呼ばれなくなると、接続数リミッタが減らないまま
// 積み上がり、ユーザーが5接続で締め出される（切断しても直らない）。
func TestWebsocketCloseFuncReleasesConnectionSlot(t *testing.T) {
	type wsConnectedKey struct{}

	limiter := connlimit.NewWSLimiter()
	closed := make(chan struct{}, 1)

	gqlServer := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{PubSub: pubsub.New()}}))
	gqlServer.AddTransport(transport.Websocket{
		Implementation: newOriginCheckingWebsocketImplementation([]string{"https://app.example.com"}),
		InitFunc: func(ctx context.Context, _ transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			if err := limiter.Acquire(1); err != nil {
				return ctx, nil, err
			}
			return context.WithValue(ctx, wsConnectedKey{}, true), nil, nil
		},
		CloseFunc: func(ctx context.Context, _ int) {
			if ctx.Value(wsConnectedKey{}) == true {
				limiter.Release(1)
			}
			select {
			case closed <- struct{}{}:
			default:
			}
		},
	})

	srv := httptest.NewServer(gqlServer)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := dialWS(t, srv, "https://app.example.com")
	if err != nil {
		t.Fatalf("接続できなかった: %v", err)
	}
	writeWS(t, ctx, conn, wsMessage{Type: "connection_init", Payload: json.RawMessage(`{}`)})
	if ack := readWS(t, ctx, conn); ack.Type != "connection_ack" {
		t.Fatalf("connection_ack が返らなかった: %+v", ack)
	}

	// InitFunc が枠を1つ使っているので、上限まで埋めたら次は取れない。
	for i := 1; i < connlimit.MaxWSConnectionsPerUser; i++ {
		if err := limiter.Acquire(1); err != nil {
			t.Fatalf("枠が想定より少ない (i=%d): %v", i, err)
		}
	}
	if err := limiter.Acquire(1); err == nil {
		t.Fatal("上限を超えて枠が取れてしまった（InitFunc が枠を確保していない）")
	}

	if err := conn.Close(coderws.StatusNormalClosure, "bye"); err != nil {
		t.Fatalf("切断に失敗した: %v", err)
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseFunc が呼ばれなかった")
	}

	// CloseFunc が枠を返したので、また1つ取れる。
	if err := limiter.Acquire(1); err != nil {
		t.Fatalf("CloseFunc が接続枠を返していない: %v", err)
	}
}
