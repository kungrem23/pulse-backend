# Keep the toolchain pinned so the same sources are built with the same Go
# release on every developer machine.
FROM golang:1.23.12-alpine3.21@sha256:4bb4be21ac98da06bc26437ee870c4973f8039f13e9a1a36971b4517632b0fc6 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/backend ./cmd/api

# The binary is static. Copy only the CA bundle needed by future HTTPS-backed
# adapters and run without root or a shell in the final image.
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/backend /backend

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/backend"]
