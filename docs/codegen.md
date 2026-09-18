# コード生成

## サーバー（gqlgen）

`graph/*.graphqls` を変えたら再生成する。

```
make generate
```

gqlgen は `go.mod` の `tool` ディレクティブで **v0.17.95 に固定**して登録してある。
CI は `make check-generated` で再生成後の生成物差分がないことも確認する。
（`graph/generated.go` はこのバージョンで生成されている）。`go get -tool` で入れてあるので
`go mod tidy` を走らせても `golang.org/x/tools` などのエントリは消えない。
`tools.go` のようなダミーファイルは不要。

**バージョンを上げるのは別作業**。v0.17.92 以降で `transport.Websocket` の API が変わっており、
`cmd/server/main.go` の追従が要る。

## クライアント

サーバーのスキーマを変えたら `SPACE-client` 側で `npm run codegen` も回すこと。
