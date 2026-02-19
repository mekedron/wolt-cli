# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/wolt-cli ./cmd/wolt-cli && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/wolt ./cmd/wolt

FROM alpine:3.22
RUN addgroup -S app && adduser -S app -G app
USER app
WORKDIR /app
COPY --from=builder /out/wolt-cli /usr/local/bin/wolt-cli
COPY --from=builder /out/wolt /usr/local/bin/wolt
ENTRYPOINT ["wolt-cli"]
CMD ["--help"]
