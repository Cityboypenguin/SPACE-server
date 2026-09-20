package pubsub

import (
	"testing"
)

type samplePayload struct {
	ID   string `json:"ID"`
	N    int32  `json:"n"`
	Flag bool   `json:"flag"`
}

type otherPayload struct {
	Name string `json:"name"`
}

// TestCodec_RoundTripsPointerValues は、ポインタで流した値がポインタのまま
// 戻ることを確かめる。受け手は型アサーションで元へ戻すので、ポインタと値を
// 取り違えると「そのトピックだけ何も届かない」という壊れ方になる。
func TestCodec_RoundTripsPointerValues(t *testing.T) {
	c := NewCodec()
	c.Register("sample", (*samplePayload)(nil))

	raw, err := c.Encode(&samplePayload{ID: "a", N: 3, Flag: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := c.Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	p, ok := got.(*samplePayload)
	if !ok {
		t.Fatalf("型が %T で戻ってきた。*samplePayload であること", got)
	}
	if p.ID != "a" || p.N != 3 || !p.Flag {
		t.Fatalf("中身が変わっている: %+v", p)
	}
}

// TestCodec_RoundTripsNonPointerValues は値で流した場合。
// 取り込み状況の購読がこの形で流している。
func TestCodec_RoundTripsNonPointerValues(t *testing.T) {
	c := NewCodec()
	c.Register("sample", samplePayload{})

	raw, err := c.Encode(samplePayload{ID: "b", N: 7})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := c.Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	v, ok := got.(samplePayload)
	if !ok {
		t.Fatalf("型が %T で戻ってきた。samplePayload（値）であること", got)
	}
	if v.ID != "b" || v.N != 7 {
		t.Fatalf("中身が変わっている: %+v", v)
	}
}

// TestCodec_RejectsUnregisteredTypes は、登録し忘れた型を黙って流さないことを
// 確かめる。流してしまうと、受け手が復元できずそのトピックだけ静かに死ぬ。
func TestCodec_RejectsUnregisteredTypes(t *testing.T) {
	c := NewCodec()
	c.Register("sample", (*samplePayload)(nil))

	if _, err := c.Encode(&otherPayload{Name: "x"}); err == nil {
		t.Fatal("未登録の型を配信しようとして通ってしまった")
	}
}

// TestCodec_DropsUnknownTypeNames は、知らない型名が届いた場合。
// 新しい subscription を足した台と古い台が混ざっている入れ替え中に起きる。
// 受け手にとっては知らないイベントなので、落とすのが正しい。
func TestCodec_DropsUnknownTypeNames(t *testing.T) {
	sender := NewCodec()
	sender.Register("newer", (*samplePayload)(nil))
	raw, err := sender.Encode(&samplePayload{ID: "x"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	receiver := NewCodec()
	receiver.Register("sample", (*samplePayload)(nil))
	if _, err := receiver.Decode(raw); err == nil {
		t.Fatal("知らない型名を受け入れてしまった")
	}
}

// TestCodec_RejectsConflictingRegistrations は配線の誤りを起動時に落とすこと。
// 黙って上書きすると「後から登録した方だけ届く」という追いにくい壊れ方になる。
func TestCodec_RejectsConflictingRegistrations(t *testing.T) {
	t.Run("同じ名前に別の型", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("名前の衝突が素通りした")
			}
		}()
		c := NewCodec()
		c.Register("sample", (*samplePayload)(nil))
		c.Register("sample", (*otherPayload)(nil))
	})

	t.Run("同じ型に別の名前", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("型の二重登録が素通りした")
			}
		}()
		c := NewCodec()
		c.Register("sample", (*samplePayload)(nil))
		c.Register("sample2", (*samplePayload)(nil))
	})

	t.Run("同じ名前・同じ型は通す", func(t *testing.T) {
		c := NewCodec()
		c.Register("sample", (*samplePayload)(nil))
		c.Register("sample", (*samplePayload)(nil))
	})
}
