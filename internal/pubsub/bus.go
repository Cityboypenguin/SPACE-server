package pubsub

// Bus は「トピックに流す・受け取る」だけの口。
//
// プロセス内の *PubSub と、Redis 経由で他の台へも届く実装（infra/redis）を
// 差し替えられるようにするために切ってある。購読側（graph/subscription_stream.go）
// も配信側（graph/chat_events.go）も、どちらが挿さっているかを知らない。
//
// 1台で動かしているうちは *PubSub で足りる。台を増やすと、ある台で送られた
// メッセージが別の台に繋がっている購読者へ届かなくなる（GraphQL の subscription は
// WebSocket なので、購読者は1台に貼り付く）ため、Redis 経由の実装に差し替える。
type Bus interface {
	Subscribe(topic string) chan interface{}
	Unsubscribe(topic string, ch chan interface{})
	Publish(topic string, data interface{})
}

var _ Bus = (*PubSub)(nil)
