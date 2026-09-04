# syntax=docker/dockerfile:1

# ---- dev ----
FROM golang:1.26-alpine AS dev
WORKDIR /app
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
