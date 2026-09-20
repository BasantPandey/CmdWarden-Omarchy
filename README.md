# CmdWarden-Omarchy

CmdWarden's Inspired Twin for Omarchy (Arch Linux + Hyprland). This repo is the
**gh-only Compat Mode Spike Vertical**: a real, minimal, end-to-end port of
Windows CmdWarden's approval-gate/vault/audit model to a single tool (`gh`) on
Omarchy, proving the whole path — Shim → Session Agent → Launcher identity →
Vault → Approval Gate → real `gh` — before it's generalized further.

See the wayfinder map for the full spec and ticket breakdown:
[CmdWarden for Omarchy: Spike Vertical spec](https://github.com/BasantPandey/CmdWarden/issues/10)
(on the main [CmdWarden](https://github.com/BasantPandey/CmdWarden) repo).

## Layout

- `cmd/cw` — the `cw` (alias `cmdwarden`) CLI entrypoint.
- `cmd/cmdwarden-agent` — the Session Agent daemon entrypoint (D-Bus service,
  socket-activated via a `systemd --user` unit).
- `internal/contracts` — shared domain types with no dependency on the CLI,
  the agent, or D-Bus — mirrors CmdWarden's own Contracts layer.
- `internal/*` — CLI-side and agent-side packages (identity resolution,
  policy, vault, shim, gh command classifier, audit trail, approval gate).
- `docs/` — research notes, prototypes, and design decisions referenced by
  the tickets above.

## Building & testing

```sh
go build ./...
go test ./...
```

CI runs `gofmt -l`, `go vet`, `go build`, and `go test` on every push (see
`.github/workflows/ci.yml`).
