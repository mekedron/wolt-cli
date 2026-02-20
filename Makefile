APP_NAME := wolt
VERSION ?= $(shell git describe --tags --always --dirty)

.PHONY: build run test race lint cover clean

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(APP_NAME) ./cmd/wolt

run:
	go run ./cmd/wolt --help

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -rf bin coverage.out
