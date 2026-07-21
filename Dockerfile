FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /hiveshare-server ./cmd/server \
 && go build -ldflags="-s -w" -o /hiveshare-mcp    ./cmd/mcp    \
 && go build -ldflags="-s -w" -o /hshare           ./cmd/hshare

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /hiveshare-server /usr/local/bin/hiveshare-server
COPY --from=builder /hiveshare-mcp    /usr/local/bin/hiveshare-mcp
COPY --from=builder /hshare           /usr/local/bin/hshare
COPY migrations/                      /app/migrations/
WORKDIR /app
EXPOSE 8080
CMD ["hiveshare-server"]
