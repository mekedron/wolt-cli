# AGENTS.MD

## Repository-Specific Git Rule

- Never set `origin` to HTTPS.
- Keep `origin` on SSH only: `git@github.com:mekedron/wolt-cli.git`.
- If `origin` is changed, immediately restore it with:
  - `git remote set-url origin git@github.com:mekedron/wolt-cli.git`

## Release Tag Rule

- Release tags must use `v` prefix only:
  - `vX.Y.Z` (for example `v0.2.0`)
