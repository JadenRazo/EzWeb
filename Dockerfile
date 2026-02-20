FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Generate templ files
RUN templ generate

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ezweb ./cmd/ezweb

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-compose tar gzip

RUN addgroup -g 1000 ezweb && adduser -u 1000 -G ezweb -D ezweb

WORKDIR /app

COPY --from=builder /app/ezweb .
COPY --from=builder /app/static ./static

RUN mkdir -p /app/backups /app/data && chown ezweb:ezweb /app/backups /app/data

USER ezweb

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8088/healthz || exit 1

ENTRYPOINT ["./ezweb"]
