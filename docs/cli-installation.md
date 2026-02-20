# Installation and Build

## Requirements

- Go `1.26+`
- Git

## Recommended: Homebrew Tap

Use the tap at [mekedron/tap](https://github.com/mekedron/tap):

```bash
brew tap mekedron/tap
brew install wolt-cli
```

Or install directly:

```bash
brew install mekedron/tap/wolt-cli
```

## Clone

```bash
git clone https://github.com/mekedron/wolt-cli.git
cd wolt-cli
```

## Build

Build all packages:

```bash
go build ./...
```

Build the CLI binary:

```bash
go build -o bin/wolt ./cmd/wolt
```

Build via Make:

```bash
make build
```

## Run

Run directly without installing:

```bash
go run ./cmd/wolt --help
```

Run built binary:

```bash
./bin/wolt --help
```

## Verify

```bash
go test ./...
```

## Optional: Lint

```bash
make lint
```

If `golangci-lint` is missing:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```
