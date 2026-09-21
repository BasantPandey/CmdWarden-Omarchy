# CmdWarden-Omarchy

**Gate what your AI harness (or anything else) can do with `gh`, without
changing `gh` itself.**

CmdWarden-Omarchy is a real, working Inspired Twin of Windows
[CmdWarden](https://github.com/BasantPandey/CmdWarden) for Omarchy (Arch
Linux + Hyprland): a Session Agent, a Launcher identity model, a policy
engine, a Secret Service vault, a real on-screen Approval Gate, and a Shim
that intercepts `gh` — wired together end to end and verified live, not just
designed on paper. See [Status](#status) for exactly what that means.

## Why

Anything you run in your terminal — a shell, an AI coding harness, a script
— inherits your full `gh` credentials with no gate in between. If it can run
`gh`, it can open PRs, change repo settings, or read a token good enough to
do either. CmdWarden-Omarchy puts a real gate there: policy decides what
auto-allows by *who's calling* and *what they're trying to do*; anything
riskier pops a real approval prompt; every decision is logged; and the
credential itself is only ever handed to a child process that was actually
approved.

## What it does

- **Resolves who's really calling** (`internal/identity`) by walking process
  ancestry to the first real Launcher and classifying its Provenance Channel
  — `mise:claude`, `pacman:foot`, or a path+hash Unmanaged identity — not a
  self-reported name a caller could lie about.
- **Decides by policy** (`internal/policy`): a Deny/Read/Trusted/Full ×
  read/write/secret-reveal/unknown auto-allow matrix, with explicit-only
  enrollment (`cw policy enroll`) and an unenrolled Launcher defaulting to
  Deny.
- **Pops a real Approval Gate** when policy doesn't auto-allow: a genuine
  Wayland layer-shell surface (Quickshell), not a stub — Deny / Approve Once
  / Allow for Session, each one actually returned to the agent and acted on.
- **Vaults gh's token** (`internal/secretservice`) in the freedesktop Secret
  Service (the same store `gh` itself uses) and releases it only into the
  one child process a gate decision just approved — never printed, never
  left in a parent shell's environment.
- **Intercepts `gh` with a Shim** (`internal/shim`) that adapts to how it's
  installed: an Occupied Shim (binary renamed aside, in place) for
  mise-managed tools, a Path Shim (PATH-prepended, real binary untouched)
  for pacman-managed ones.
- **Logs every decision** (`internal/audit`) to an NDJSON trail that
  structurally cannot carry a secret value or a full command line, and fails
  closed if the log itself can't be written.

Read [`docs/architecture.md`](docs/architecture.md) for how these fit
together, or [`docs/cli-reference.md`](docs/cli-reference.md) for every `cw`
command.

## Status

This repo is the **gh-only Compat Mode Spike Vertical**: all 11 tickets on
the [wayfinder map](https://github.com/BasantPandey/CmdWarden/issues/10) are
closed, and the full path — Shim → Session Agent → Launcher identity →
Vault → Approval Gate → real `gh` — has been verified live on a real Omarchy
machine against a real, actively-used `gh` install, including: Deny actually
blocking a real `gh pr create`-style command; Approve Once allowing exactly
one call; Allow for Session persisting across a second call with zero
re-prompt; and a correctly-classified audit row for every one of those. It
is **one tool** (`gh`), proving the model before generalizing it — see
[Explicit non-goals](#explicit-non-goals-v1).

## Platform support

Developed against and tested on **Omarchy** (Arch Linux + Hyprland), but the
design is mostly generic **systemd + D-Bus + wlroots-Wayland Linux** —
Omarchy is where it's proven, not an architectural requirement. The one
Omarchy/Arch-specific piece is `pacman`-based Provenance Channel detection,
which degrades gracefully (falls back to path+hash Unmanaged identity)
anywhere `pacman` isn't the package manager. See
[Platform requirements and portability](docs/architecture.md#platform-requirements-and-portability)
for the full breakdown.

Requires: Linux, `systemd --user`, a D-Bus session bus, a freedesktop Secret
Service provider (gnome-keyring, KWallet, ...), a wlr-layer-shell-capable
Wayland compositor (Hyprland, Sway, ...) and Quickshell for the Approval
Gate, and `gh` itself.

## Quick start

```sh
go build -o bin/cw ./cmd/cw
go build -o bin/cmdwarden-agent ./cmd/cmdwarden-agent

./bin/cw agent install        # writes + enables the systemd --user socket unit
./bin/cw doctor                # lazy-starts the agent, confirms it's healthy

./bin/cw policy enroll --kind ai-harness   # enroll the calling Launcher (defaults to Read)
./bin/cw harden gh                          # import gh's token, install the Shim

gh pr create ...                            # now gated
```

Full command reference: [`docs/cli-reference.md`](docs/cli-reference.md).

## Layout

- `cmd/cw` — the `cw` (alias `cmdwarden`) CLI entrypoint.
- `cmd/cmdwarden-agent` — the Session Agent daemon entrypoint (D-Bus service,
  socket-activated via a `systemd --user` unit).
- `internal/contracts` — shared domain types with no dependency on the CLI,
  the agent, or D-Bus — mirrors CmdWarden's own Contracts layer.
- `internal/*` — CLI-side and agent-side packages (identity resolution,
  policy, vault, shim, gh command classifier, audit trail, approval gate).
- `docs/` — architecture, CLI reference, and the research/prototype records
  that informed the design.

## Building & testing

```sh
go build ./...
go test ./...
```

CI runs `gofmt -l`, `go vet`, `go build`, and `go test` on every push (see
`.github/workflows/ci.yml`); `.github/workflows/release.yml` builds and
publishes versioned `linux/amd64` and `linux/arm64` packages on a `v*.*.*`
tag push.

## Explicit non-goals (v1)

This spike vertical deliberately covers `gh` only. The Windows CmdWarden
spec's own catalog (git, az, docker) and hardening depth (strong mode,
credential-store migration) are out of scope here — see
[the wayfinder map](https://github.com/BasantPandey/CmdWarden/issues/10) for
what comes after proving the model on one tool.
