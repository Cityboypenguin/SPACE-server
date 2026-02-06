# syntax=docker/dockerfile:1

FROM golang:1.25-alpine

WORKDIR /app

# air をインストール（開発用）
RUN go install github.com/air-verse/air@latest

# 依存解決を後で最適化したい場合は go.mod/go.sum を先にCOPYしますが、
# 最小構成なので一旦省略し、ソースは volume mount 前提で空のままにします。

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
