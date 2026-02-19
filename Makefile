APP_NAME := wolt-cli
ALT_APP_NAME := wolt

.PHONY: build run test race lint cover clean

build:
	go build -o bin/$(APP_NAME) ./cmd/wolt-cli
	go build -o bin/$(ALT_APP_NAME) ./cmd/wolt

run:
	go run ./cmd/wolt-cli --help

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
