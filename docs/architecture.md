# Architecture

CmdWarden-Omarchy gates a developer's use of `gh` behind a policy check and,
when policy doesn't auto-allow, a real on-screen Approval Gate — without
requiring `gh` itself to change. It does this the same way Windows
CmdWarden does: intercept the binary with a Shim, decide via a long-lived
Session Agent, and only ever hand a live credential to a child process that
was actually approved.

## Why

An AI coding harness (or any automated tool) running in your terminal has
the same `gh` credentials you do. If it can run `gh`, it can create PRs,
change repo settings, or read a token good enough to do either — with no
gate in between. CmdWarden-Omarchy puts one there: gate write/secret-reveal
`gh` calls behind an approval, gate credential release behind the same
approval, and log every decision.

## Components

```
                    ┌─────────────────────────┐
   your shell ──▶   │   gh (Shim, tiny script)│
   (bash, an AI     └───────────┬─────────────┘
    harness, ...)               │ exec
                                 ▼
                    ┌─────────────────────────┐
                    │ cw shim-exec            │  resolves identity, classifies
                    │ (internal/shimexec)     │  the command, asks policy
                    └───────────┬─────────────┘
                                 │ D-Bus (session bus)
                                 ▼
                    ┌─────────────────────────┐
                    │ cmdwarden-agent          │  identity resolution, policy
                    │ (internal/agentd)        │  store, vault, audit, gate
                    │ socket-activated by      │  orchestration, session grants
                    │ systemd --user           │
                    └───────────┬─────────────┘
                       │        │        │
              (prompt) │        │ (release)
                       ▼        │        ▼
        ┌───────────────────┐  │  ┌───────────────────────┐
        │ Approval Gate UI   │  │  │ Secret Service vault  │
        │ (Quickshell, real  │  │  │ (gnome-keyring/KWallet │
        │ Wayland layer-     │  │  │  via org.freedesktop.  │
        │ shell surface)     │  │  │  secrets)              │
        └───────────────────┘  │  └───────────────────────┘
                                 ▼
                    ┌─────────────────────────┐
                    │ real gh binary           │  exec'd with GH_TOKEN from
                    │ (renamed aside by the    │  the vault injected, only
                    │  Shim installer)         │  if the decision was allow
                    └─────────────────────────┘
```

Every gate decision writes one row to the audit log
(`~/.local/state/cmdwarden/gate-decisions.ndjson`) before the command is
allowed to proceed — see [Audit](#audit) below.

## Launcher identity (`internal/identity`)

Every policy decision is keyed on **who's calling** — the Identity Key,
`<provenance channel>:<tool>` (e.g. `mise:claude`, `pacman:foot`). It's
resolved by walking `/proc` ancestry from the real D-Bus caller's PID
(obtained via `GetConnectionUnixProcessID`, never self-reported — a lying
client can't spoof this) up to the first ancestor that isn't a generic
shell/wrapper process. That ancestor's binary is then classified by
**Provenance Channel**:

- **mise** — the resolved path lives under `~/.local/share/mise/installs/`;
  the tool name is the mise plugin name, which stays stable across a
  version bump (mise repoints a `latest` symlink rather than mutating a
  path in place).
- **pacman** — `pacman -Qo <path>` reports an owning package; the tool name
  is that package.
- **Unmanaged** — neither: identity falls back to a path + SHA-256 hash of
  the binary's own contents.

A resolved Launcher also carries its PID and start time (`identity.Launcher`)
— used only for session-grant tracking (see [Session grants](#session-grants)),
never for the identity itself.

## Policy (`internal/policy`)

A 4×4 auto-allow matrix, evaluated per (level, command class):

| Level \ Class | read | write | secret-reveal | unknown |
|---|---|---|---|---|
| **Deny** | no | no | no | no |
| **Read** | yes | no | no | no |
| **Trusted** | yes | yes | no | no |
| **Full** | yes | yes | yes | yes |

Enrollment is **explicit only** — `cw policy enroll --kind ai-harness\|terminal`
— with no silent auto-enroll; an unenrolled Identity Key defaults to `Deny`.
`Deny` is a hard stop: it never prompts (see `policy.Decide`'s doc comment)
— a policy level of Deny means "never even ask," not "ask every time."
Anything not auto-allowed falls through to the Approval Gate.

## Command classification (`internal/ghclassify`)

A pure lookup table mapping a `gh` subcommand to a Command Class:
token-export commands (`auth token`, `auth git-credential get`, `api`) are
**secret-reveal**; side-effecting or auth-state-changing commands (`pr
create`, `repo create`, `auth login`) are **write**; read-mostly commands
(`repo view`, `pr list`) are **read**; anything unrecognized is **unknown**
— which only auto-allows under `Full`, never silently treated as `read`.

## Vault (`internal/secretservice`, `internal/agentd`'s vault methods)

A minimal freedesktop Secret Service client (the same D-Bus API `gh` itself
uses via `zalando/go-keyring`). `cw harden gh` imports `gh`'s currently
active token — checked in the same order `gh` itself resolves a token:
`GH_TOKEN`/`GITHUB_TOKEN`, then `hosts.yml`'s plaintext `oauth_token`, then
gh's own `gh:<host>` Secret Service entry — into a CmdWarden-owned vault
item. On every allowed `gh` call, the Shim's dispatcher releases that token
and injects it as `GH_TOKEN` into *that one child process's* environment —
never printed, never left in a parent shell's environment.

## Shim (`internal/shim`)

Installing over a tool branches on its Provenance Channel:

- **Occupied Shim** (mise) — the real binary is renamed aside to
  `<path>.cmdwarden-real` and a tiny script takes its exact file path.
  Every resolution path that used to reach the real binary now reaches the
  Shim, no PATH tricks involved. A mise version bump orphans the pin (a
  fresh, unshimmed binary appears at a new path) — `cw doctor`'s **Pin
  Drift** check catches this, and re-running `cw harden gh` is always safe
  (it removes a stale pin before reinstalling).
- **Path Shim** (pacman) — the real binary is never touched. A same-named
  script goes into `~/.local/share/cmdwarden/shims/`, prepended on `PATH`
  via a per-user bootstrap (`~/.bashrc`, `~/.profile`, `~/.config/uwsm/env.d/`,
  and `~/.config/environment.d/`'s `BASH_ENV` for non-interactive shells,
  which don't read `~/.bashrc` at all) — the closest per-user equivalent of
  Omarchy's own PAM-level PATH injection for SSH sessions, since cw has no
  root.

Every Shim script is identical regardless of shape:
`exec cw shim-exec --tool NAME --real PATH -- "$@"`. The tool name and real
path are baked in literally at install time — not inferred from `$0`, which
a shell sets to whatever bare name the user typed, not a resolved path, for
anything found via `PATH` search.

## Approval Gate (`internal/agentd/qml`, ticket #6)

A standalone Quickshell instance (`qs -p`, embedded QML extracted to a temp
dir per request) — not an omarchy-shell plugin, so it doesn't depend on
omarchy-shell's internal theme singletons or plugin registry staying stable.
A single `PanelWindow` on `WlrLayer.Overlay` with exclusive keyboard focus.
Deny / Approve Once / Allow for Session each run `cw gate respond` (a plain
argv `Process`, waited on to completion) before closing, so the window can
never close without the agent having heard the answer. If `qs` isn't on
`PATH`, there's no `WAYLAND_DISPLAY`, the process exits without answering,
or a 5-minute timeout elapses — it fails closed (`Unavailable`), never
hangs and never silently allows.

## Session grants (`internal/agentd/session.go`)

"Allow for Session" is tracked in the agent's memory, keyed by the
Launcher's PID **and start time** (so a reused PID never inherits someone
else's grant): it covers the granted class and anything less privileged
(`write` also covers `read`) for that tool, until the launcher process
exits or the grant goes idle (`CW_SESSION_IDLE_SECONDS`, default 3600s).
Checked before ever popping the UI — a covered call returns
`session-allow` instantly, with no prompt.

## Audit

An NDJSON log (`~/.local/state/cmdwarden/gate-decisions.ndjson`), one row
per gate decision (`auto-allow`, `allow-once`, `deny`, `unavailable`,
`session-grant`, `session-allow`), with a fixed field set that structurally
cannot carry a secret value or a full command line. `audit.Log` fsyncs and
returns an error on any failure; every call site treats that as fail-closed
— a decision that can't be durably audited doesn't get to proceed, even if
it was otherwise an allow.

## Transport: the Session Agent

`cmdwarden-agent` has two transports:

- A **systemd `--user` socket-activated Unix socket**
  (`cmdwarden-agent.socket`) — its only job is giving systemd something to
  lazily start the service on, and answering a trivial `PING`/`STOP`
  liveness check even before the D-Bus registration below completes.
- The **real D-Bus session bus** — the agent requests
  `org.cmdwarden.Agent1` and exports every real RPC
  (`ResolveIdentity`, `RequestGate`, `SubmitGateDecision`, `SaveSecret`,
  `ReleaseSecret`, `DeleteSecret`, `ImportGHToken`) at
  `/org/cmdwarden/Agent1`.

`cw doctor` lazy-starts the agent (connects the socket, which systemd turns
into a fresh process if none is running yet) and polls the D-Bus name until
it answers healthy.

## Platform requirements and portability

Built against and tested on **Omarchy** (Arch Linux + Hyprland), but most of
the design isn't Omarchy-specific:

| Requirement | Where it's used | Portable beyond Omarchy? |
|---|---|---|
| `systemd --user` | Session Agent socket activation | Yes — any systemd-based distro |
| D-Bus session bus | Agent RPC, Secret Service | Yes — standard on any Linux desktop |
| freedesktop Secret Service | Vault | Yes — gnome-keyring, KWallet, keepassxc all implement it |
| Wayland + `wlr-layer-shell` | Approval Gate UI | Yes on wlroots compositors (Hyprland, Sway, ...); not X11, and not guaranteed on non-wlroots compositors (GNOME/KDE have partial/varying layer-shell support) |
| `pacman` | Provenance Channel detection | **No** — Arch-specific. Gracefully degrades: a non-pacman system just never resolves the `pacman:*` channel, falling back to Unmanaged (path+hash) identity instead of misclassifying anything |
| `mise` | Provenance Channel detection | Yes — mise itself is cross-distro |

In short: this is a **generic systemd + D-Bus + wlroots-Wayland Linux**
design, developed and proven specifically on Omarchy. Running it on another
Arch+wlroots setup should work unmodified; running it on a non-Arch,
non-wlroots Linux desktop would still get identity/policy/vault/audit
working correctly, but Launcher identity would never resolve `pacman:*`
(everything not mise-managed becomes Unmanaged), and the Approval Gate would
need a wlr-layer-shell-capable compositor to render at all.

## See also

- [`docs/research/gh-linux-keyring.md`](research/gh-linux-keyring.md) — how
  `gh` itself stores and resolves its token on Linux, which the vault design
  (`ImportGHToken`) mirrors exactly.
- [`docs/research/omarchy-shell-ipc.md`](research/omarchy-shell-ipc.md) — why
  the Approval Gate is a standalone Quickshell instance rather than an
  omarchy-shell plugin or a `org.freedesktop.Notifications` toast.
- [`docs/prototypes/approval-gate/`](prototypes/approval-gate/) — the
  throwaway UI-variant prototype that settled on the Center Modal chrome.
- [`docs/cli-reference.md`](cli-reference.md) — every `cw` command.
