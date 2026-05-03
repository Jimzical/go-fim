# Multi-stage build for the go-fim agent. Build stage uses the full Go
# toolchain; runtime is alpine (~7 MB) because the wrapper loop needs a
# shell — distroless static would be smaller but precludes `sh -c`.

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd
COPY internal/ ./internal
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/go-fim ./cmd/go-fim

FROM alpine:3.20
RUN adduser -D -u 1000 fim && mkdir -p /data && chown fim:fim /data
COPY --from=build /out/go-fim /usr/local/bin/go-fim
USER fim
# /data exists in the image owned by fim; Docker propagates that ownership
# into a fresh named volume on first mount, so bbolt can write snapshot.db.
# One-shot agent + sleep loop = poor-man's cron inside the container.
CMD ["sh", "-c", "while true; do /usr/local/bin/go-fim -c /etc/gofim/gofim.yml; sleep 30; done"]
