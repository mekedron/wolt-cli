# AGENTS.MD

## Repository-Specific Git Rule

- Never set `origin` to HTTPS.
- Keep `origin` on SSH only: `git@github.com:mekedron/wolt-cli.git`.
- If `origin` is changed, immediately restore it with:
  - `git remote set-url origin git@github.com:mekedron/wolt-cli.git`

## Release Tag Rule

- Release tags must use `v` prefix only:
  - `vX.Y.Z` (for example `v0.2.0`)

## Commit Message Rule

- Commit subjects must describe real code/content changes.
- Do not use placeholder subjects like `feat: release vX.Y.Z` without change context.

## Pre-Push Validation Rule

- Before every `git push`, run local CI-equivalent checks and tests.
- Minimum required commands:
  - `go mod download`
  - `go build ./...`
  - `BUILD_VERSION="$(git describe --tags --always --dirty)" && go build -trimpath -ldflags "-s -w -X main.version=${BUILD_VERSION}" -o bin/wolt ./cmd/wolt`
  - `test "$(./bin/wolt --version | tr -d '\r\n')" = "${BUILD_VERSION}"`
  - `golangci-lint run`
  - `go test ./...`
  - `go test -race ./...`
