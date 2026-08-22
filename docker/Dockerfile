# syntax=docker/dockerfile:1

# ---- dev ----
FROM golang:1.25-alpine AS dev
WORKDIR /app
# curl is required by cmd/scraper: the target syllabus site only offers a classic
# DHE (non-ECDHE) TLS cipher suite, which Go's crypto/tls does not implement, so
# that command shells out to curl (backed by OpenSSL) instead of net/http.
RUN apk add --no-cache curl
RUN go install github.com/air-verse/air@latest
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# ---- builder ----
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- prod ----
FROM alpine:3.21 AS prod
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
