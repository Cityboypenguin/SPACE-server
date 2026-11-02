package graph

import (
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type threadChangeBus struct{ topics, payloads []string }

func (b *threadChangeBus) Publish(topic string, data interface{}) {
	b.topics = append(b.topics, topic)
	b.payloads = append(b.payloads, data.(*ChatThreadChanged).ThreadID)
}

func TestPublishChatThreadChanged(t *testing.T) {
	bus := &threadChangeBus{}
	parent := int64(10)
	publishChatThreadChanged(bus, &model.Room{ID: 20, ParentRoomID: &parent})
	// スレッドでないルームでは知らせない。
	publishChatThreadChanged(bus, &model.Room{ID: 10})
	publishChatThreadChanged(bus, nil)

	if len(bus.topics) != 1 {
		t.Fatalf("published %d times, want 1", len(bus.topics))
	}
	if bus.topics[0] != chatThreadChangedTopic(10) || bus.payloads[0] != encodeGraphID("room", 20) {
		t.Fatalf("got %q %q", bus.topics[0], bus.payloads[0])
	}
}
