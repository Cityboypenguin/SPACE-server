package pubsub

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// Codec は PubSub に流す値を、プロセスの外へ出せる形（JSON）へ直す。
//
// プロセス内の PubSub は interface{} をそのまま渡していたので、受け手は型アサーションで
// 元の型に戻せた。台をまたぐとそうはいかない。届くのはバイト列だけで、それが
// *gqlmodel.Message なのか *gqlmodel.Poll なのかは書いておかないと分からない。
//
// そこで「型の名前」を添えて送り、受け手は名前から具体型を引いてそこへ復元する。
// 名前は登録時に明示する（Go の型名をそのまま使うと、型の移動やリネームで
// 配信が静かに壊れる。壊れ方は「そのトピックだけ何も届かない」なので気づきにくい）。
//
// 登録するのは PubSub に流す型だけでよい。登録漏れは Encode/Decode がエラーを返すので、
// 黙って落ちることはない。
type Codec struct {
	mu     sync.RWMutex
	byName map[string]reflect.Type
	byType map[reflect.Type]string
}

func NewCodec() *Codec {
	return &Codec{
		byName: make(map[string]reflect.Type),
		byType: make(map[reflect.Type]string),
	}
}

// Register は name とポインタ型の対応を覚える。sample は型を取るためだけに使う
// （例: (*gqlmodel.Message)(nil)）。
//
// 同じ名前・同じ型を二重に登録するのは panic にしている。配線の誤りは起動時に
// 分かるのが一番安く、黙って上書きすると「後から登録した方だけ届く」という
// 追いにくい壊れ方になる。
func (c *Codec) Register(name string, sample any) {
	t := reflect.TypeOf(sample)
	if t == nil {
		panic("pubsub: Register には型を取れる値を渡すこと")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.byName[name]; ok && existing != t {
		panic(fmt.Sprintf("pubsub: 名前 %q は既に %s に使われている", name, existing))
	}
	if existing, ok := c.byType[t]; ok && existing != name {
		panic(fmt.Sprintf("pubsub: 型 %s は既に %q で登録されている", t, existing))
	}
	c.byName[name] = t
	c.byType[t] = name
}

// envelope は流すバイト列の形。
type envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (c *Codec) Encode(data any) ([]byte, error) {
	t := reflect.TypeOf(data)
	c.mu.RLock()
	name, ok := c.byType[t]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pubsub: 未登録の型 %s は配信できない", t)
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("pubsub: %s の変換に失敗: %w", name, err)
	}
	return json.Marshal(envelope{Type: name, Payload: payload})
}

func (c *Codec) Decode(raw []byte) (any, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("pubsub: 受け取った値の形が違う: %w", err)
	}

	c.mu.RLock()
	t, ok := c.byName[env.Type]
	c.mu.RUnlock()
	if !ok {
		// 新しい型を足した台と古い台が混ざっている（入れ替え中など）。
		// 受け手にとっては「知らないイベント」なので、落とすのが正しい。
		return nil, fmt.Errorf("pubsub: 未登録の型名 %q", env.Type)
	}

	// 送り手が渡したのと同じ形（ポインタで送ったならポインタ、値なら値）で返す。
	// 受け手は型アサーションで元に戻すので、ここでポインタと値を取り違えると
	// アサーションが通らず、そのトピックだけ静かに届かなくなる。
	if t.Kind() == reflect.Pointer {
		value := reflect.New(t.Elem())
		if err := json.Unmarshal(env.Payload, value.Interface()); err != nil {
			return nil, fmt.Errorf("pubsub: %s の復元に失敗: %w", env.Type, err)
		}
		return value.Interface(), nil
	}
	value := reflect.New(t)
	if err := json.Unmarshal(env.Payload, value.Interface()); err != nil {
		return nil, fmt.Errorf("pubsub: %s の復元に失敗: %w", env.Type, err)
	}
	return value.Elem().Interface(), nil
}
