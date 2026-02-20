# AGENTS.MD

## Repository-Specific Git Rule

- Never set `origin` to HTTPS.
- Keep `origin` on SSH only: `git@github.com:mekedron/wolt-cli.git`.
- If `origin` is changed, immediately restore it with:
  - `git remote set-url origin git@github.com:mekedron/wolt-cli.git`

## Release Tag Rule

- GitHub release workflow must support both tag styles:
  - `vX.Y.Z` (for example `v0.2.0`)
  - `X.Y.Z` (for example `0.2.0`)
