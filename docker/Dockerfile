# syntax=docker/dockerfile:1

# ---- dev ----
FROM golang:1.27-alpine AS dev
WORKDIR /app
RUN apk add --no-cache curl
RUN go install github.com/air-verse/air@v1.67.4
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# ---- builder ----
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- prod ----
FROM alpine:3.21 AS prod
WORKDIR /app
RUN apk add --no-cache curl
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
