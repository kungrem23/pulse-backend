# Образ для сборки
FROM golang:1.23.12-alpine3.21@sha256:4bb4be21ac98da06bc26437ee870c4973f8039f13e9a1a36971b4517632b0fc6 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o backend ./cmd/api/main.go

# Образ для запуска приложения
FROM debian:12-slim

WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/src/app/backend .
# COPY .env .

CMD [ "./backend" ]
