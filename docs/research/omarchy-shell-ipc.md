# Research: omarchy-shell plugin IPC surface for showing a prompt

Resolves the investigation portion of [issue #12](https://github.com/BasantPandey/CmdWarden/issues/12)
(blocks #15, the Approval Gate UI prototype ticket). This is groundwork, not a
final design decision — it answers what the platform offers; #15 decides what
CmdWarden-Omarchy builds on top of it.

## Machine facts (ground truth, gathered on a real Omarchy install)

- `omarchy-shell` binary: `/usr/bin/omarchy-shell` → `/usr/share/omarchy/bin/omarchy-shell`
  (thin bash wrapper). Config/plugin tree: `/usr/share/omarchy/shell/`.
- Installed as package `try-omarchy-runtime 4.0.2-1`
  (`provides: omarchy=4.0.2`), described as "Pinned Basecamp Omarchy runtime
  `346e69e1cec6c4e8924531874af6ba010a1bc99e` for the Try Omarchy guest",
  URL `https://github.com/basecamp/omarchy`. So this machine runs a pinned
  snapshot of upstream Omarchy at that commit, not a rolling install.
- `omarchy-shell` is one long-running **Quickshell** (quickshell.org, QML-based
  Wayland shell toolkit) process — `qs` binary, Quickshell 0.3.1. Confirmed live:
  `busctl --user list` shows PID 737 (`quickshell`) owning `:1.12`/`:1.20`/`:1.21`/etc.
- There is no mako/dunst/swaync on this machine, confirmed by `busctl --user list`
  and by the absence of the packages.

## 1. The plugin IPC protocol — documented, and empirically verified working here

`omarchy-shell`'s own in-tree docs (`/usr/share/omarchy/shell/README.md` and
`/usr/share/omarchy/shell/plugins/README.md`, part of the installed package)
fully document:

- **Plugin manifest schema** (`manifest.json`: `schemaVersion`, `id`, `kinds`,
  `entryPoints`, plus kind-specific blocks). Supported `kinds`: `bar-widget`,
  `panel`, `overlay`, `menu`, `service`, `bar`.
- **The `shell` IPC target**, reachable via Quickshell's own IPC mechanism —
  **not D-Bus**. Transport is `qs ipc -p $OMARCHY_PATH/shell call -- <target>
  <method> [args...]`, which `omarchy-shell` wraps:

  | Method | Returns | Effect |
  |---|---|---|
  | `ping` | `ok` | health check |
  | `summon <id> <payloadJson>` | `ok`/`unknown` | load + open a panel/overlay/menu plugin |
  | `hide <id>` | — | close a previously-summoned plugin |
  | `toggle <id> <payloadJson>` | — | summon if closed, hide if open |
  | `call <id> <method> <arg>` | string | call a method on an **already-loaded** plugin |
  | `rescanPlugins` | — | re-walk plugin dirs, hot-reload plugin code |
  | `reloadConfig` | `ok` | reload `~/.config/omarchy/shell.json` |
  | `setPluginEnabled <id> <enabled>` | `ok`/`unknown` | flip persisted enabled bit |
  | `listPlugins` | JSON | every discovered plugin, sorted by name |

  Empirically verified live on this machine:

  ```
  $ omarchy-shell shell listPlugins
  [{"id":"omarchy.active-window", ...}, {"id":"omarchy.agents", ...}, ...]
  ```

  returns the full first-party plugin roster with `enabled`/`active`/`firstParty`
  flags — the IPC surface genuinely works, is reachable from an ordinary shell
  command, and requires no special privilege beyond being in the same Wayland
  session (the wrapper recovers `WAYLAND_DISPLAY` from the compositor socket
  when unset, e.g. for an SSH/TTY caller).

- **Third-party plugin installation** is a documented, scriptable workflow:
  a plugin is a git repo with `manifest.json` at its root; `omarchy plugin add
  <url> --enable --yes` clones it into `~/.config/omarchy/plugins/<id>/` and
  enables it non-interactively. The docs explicitly say: *"Pass `--yes` to
  skip every prompt — this is the path for scripts and AI agents."* Plugins run
  **unsandboxed inside the omarchy-shell process** (the docs warn about this).
  A plugin can also be dropped in by hand (copy files + `rescanPlugins` +
  `omarchy plugin enable <id>`) with no git involved at all.

### (a) Can a third-party, non-plugin process show custom UI via this IPC?

**Two different capabilities, worth separating:**

1. **Calling existing IPC methods (`ping`, `summon`, `toggle`, `listPlugins`,
   etc.) needs no registration at all.** Any process in the session can shell
   out to `omarchy-shell shell <method> ...` right now, as demonstrated above.
2. **Showing genuinely new/custom UI content does require being a loaded
   plugin.** `summon <id>` and `call <id> <method> <arg>` only operate on a
   plugin `id` the shell has already discovered (first-party, or dropped into
   `~/.config/omarchy/plugins/`). There is no IPC method to hand the shell an
   arbitrary one-off QML payload to render — the shell's IPC surface **routes
   to plugin code**, it doesn't **define UI inline**. So a CmdWarden Session
   Agent cannot pop a custom Deny / Allow-once / Allow-for-session card purely
   by calling IPC methods against the stock shell; it would need to ship a
   small `panel`- or `service`-kind plugin (QML + `manifest.json`) and install
   it via the documented `omarchy plugin add --yes` (or hand-drop) path, then
   `summon`/`call` into its own plugin id. This is a real, low-friction,
   scriptable path — explicitly designed for automated/agent installation —
   but it is a **one-time setup step** (installing the CmdWarden plugin),
   not something achievable from an uninstalled, ordinary process.

## 2. Is there a compliant interactive notification server? Yes — but its shipped UI doesn't render action buttons

Confirmed via `busctl --user list` / `introspect`:

```
org.freedesktop.Notifications   737 quickshell  basant :1.12  user@1000.service
```

`quickshell` itself (i.e., `omarchy-shell`'s own first-party `omarchy.notifications`
service plugin, `/usr/share/omarchy/shell/plugins/notifications/`) owns the
well-known name `org.freedesktop.Notifications` on the session bus — there is
no mako/dunst/swaync involved, and `notify-send`/`omarchy-notification-send`
talk directly to this.

`GetCapabilities` returns:

```
"persistence" "body" "body-markup" "body-hyperlinks" "actions" "icon-static"
```

So the server **advertises** the `actions` capability (the spec's signal that
a client may pass named actions to `Notify()` and expect them wired up), and
its `Notify()`/`ActionInvoked`/`NotificationClosed`/`NotificationReplied`
D-Bus methods/signals are all present per `busctl --user introspect`.

**However**, reading the actual UI implementation
(`plugins/notifications/components/NotificationCard.qml` and
`plugins/notifications/Service.qml`) shows the shipped toast/history card only
wires up:

- **left-click → the single `"default"` action** (if the sender registered
  one; `Service.qml`'s `invokePopupDefault()` walks `ref.actions` looking only
  for `identifier === "default"`), and
- **right-click / hover-revealed X → close/dismiss.**

There is no `Repeater`/button-row rendering multiple named actions in the
card component. So today, a caller *can* set an `actions` array on `Notify()`
without erroring, but the stock notification popup gives the user no way to
choose among more than the one default action — **a real Deny / Allow-once /
Allow-for-session three-way choice is not renderable through Omarchy's stock
notification popup as shipped**, even though the underlying protocol
plumbing (and capability advertisement) is there. This is a dead end for the
Approval Gate's actual interaction model, though it may still be useful for a
low-stakes "heads up" toast that links out to a real approval surface.

## 3. Minimum-friction path to a real interactive modal

Given (1) and (2), a bespoke interactive modal is the right shape for the
Approval Gate, and Hyprland's compositor model supports this natively via the
**wlr-layer-shell** Wayland protocol (`zwlr_layer_shell_v1`), which is exactly
what `omarchy-shell` itself uses for its own overlays. Evidence from
`plugins/polkit/PolkitAgent.qml` (Omarchy's own themed polkit authentication
dialog — the closest existing prior art to an "approve/deny" modal):

```qml
import Quickshell.Wayland
...
PanelWindow {
  WlrLayershell.namespace: "omarchy-polkit"
  WlrLayershell.layer: WlrLayer.Overlay
  WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
  ...
}
```

i.e. a focus-grabbing, overlay-layer, always-on-top window — precisely the
affordance an approval prompt needs (it must be seen and interacted with
before the gated command proceeds).

Three viable implementation routes, in order of integration depth:

1. **Become an omarchy-shell plugin** (`kind: "service"` or `"panel"`,
   modeled directly on `omarchy.polkit`). Deepest integration: reuses the
   shell's theme singleton (`qs.Commons`, `qs.Ui`), runs in the same process,
   installable via the documented, agent-friendly `omarchy plugin add --yes`
   flow. Cost: requires learning Quickshell's QML/JS plugin model and coupling
   to omarchy-shell internals (theme API, IPC method plumbing) that aren't
   guaranteed stable (see upstream-maturity note below).
2. **A standalone Quickshell instance**, run as its own process
   (`qs -p <config-dir>`) using the same `Quickshell.Wayland`
   (`PanelWindow`/`WlrLayershell`) QML API `omarchy-shell` itself uses, but
   entirely independent of the running shell — no plugin registration, no
   coupling to omarchy-shell's plugin registry or theme internals. Needs
   Quickshell to be present (it is, as an omarchy-shell dependency — confirmed
   via `pacman -Qi quickshell`), but doesn't require the CmdWarden process to
   integrate with omarchy-shell's IPC at all.
3. **A standalone GTK4 layer-shell popup**, using `gtk4-layer-shell`
   (`extra/gtk4-layer-shell 1.3.0-1`) — **confirmed already installed on this
   machine** (`pacman -Qi` shows `[installed]`; the GTK3 `gtk-layer-shell` is
   available in `extra/` but not installed here). This is the most portable,
   least-coupled fallback: no dependency on Quickshell or omarchy-shell at
   all, just Hyprland's native wlr-layer-shell support (which Hyprland ships
   unconditionally). Buttons/urgency styling are entirely under CmdWarden's
   own control, matching the design freedom of the native Windows desktop
   card mentioned in the ticket.

**Recommendation for the #15 prototype ticket:** route 3 (standalone
GTK4 + gtk-layer-shell) or route 2 (standalone Quickshell instance) are the
lower-risk starting points — full UI control, no dependency on omarchy-shell's
plugin internals staying stable, and gtk4-layer-shell is already present on a
stock Omarchy install. Route 1 (a real omarchy-shell plugin) is worth
revisiting later for a more native feel (theme-matching, single always-running
process) once the plugin API's stability is better understood — see the
upstream-maturity note below.

## Upstream (external) verification

**Org rename note:** `github.com/basecamp/omarchy` (the URL recorded in this
machine's package metadata) has moved to **`github.com/omacom/omarchy`** — an
org rename, not a fork (upstream PRs such as omacom/omarchy#9080/#9087/#9088
explicitly update leftover `basecamp/omarchy` references to
`omacom/omarchy`). The old URL still redirects as of this writing; new links
should point at `omacom/omarchy` and `omarchy.org`.

- **Publicly documented, matching the local findings closely.**
  `shell/README.md` on the `quattro` branch —
  https://github.com/omacom/omarchy/blob/quattro/shell/README.md — documents
  the same `manifest.json` schema, the same `shell` IPC methods
  (`ping`/`summon`/`hide`/`toggle`/`call`/`rescanPlugins`/`reloadConfig`/
  `setPluginEnabled`/`listPlugins`), and `omarchy plugin add <url> --enable
  --yes` with the identical line *"Pass `--yes` to skip every prompt — this is
  the path for scripts and AI agents."* Also rendered at
  `omarchy.org/manual/shell-plugins/`.
- **How new is this:** the Quickshell-based shell shipped as **v4.0.0
  "Quattro"** on **2026-08-14** (github.com/omacom/omarchy/releases), i.e.
  roughly five weeks old as of this research (2026-09-20), with 4.0.1–4.0.4
  following through Sept 15 as point releases (including security hardening —
  see below). The origin PR is dhh's "Omarchy goes Quickshell"
  (github.com/omacom/omarchy/pull/5856, 627 commits merged to `quattro`). A
  corroborating write-up (codetocloud.io/blog/omarchy-4-quattro-whats-new)
  notes the QML shell is "104 QML files ... none of which existed a year
  ago." **Conclusion: young but actively-patched — treat the plugin API as
  plausible but not yet battle-hardened, and expect it to keep moving.**
- **Third-party plugins exist, not just hypothetically.** A community
  marketplace repo, github.com/omacom/omarchy-plugin-marketplace (293 stars,
  48 forks), curates a `registry.json` of submissions (mostly `[Plugin]:
  <name>` issues, e.g. "Omarchy Launcher", "Omarchy Notify", "Multi-Account
  Agent Manager", "Notification Settings"), browsable at
  omarchyplugins.com. Individual third-party plugin repos found via GitHub
  search include `huyhuyvu01/omarchy-clipboard`,
  `muhamm-ad-ahmad/omarchy-cursor`, `chupre/omaview`,
  `stappmus/Omarchy-Spotify` — real, separate git repos each with their own
  `manifest.json`. This meaningfully de-risks route 1 (shipping CmdWarden as
  an installable omarchy-shell plugin) as a *later* option, once the API
  settles.
- **mako/dunst/swaync replacement rationale:** stated in the PR #5856
  description and a corroborating blog post as consolidating "eight separate
  programs, each with its own config format, its own theming story" into one
  Quickshell process. No dedicated upstream issue/ADR titled around that
  decision was found (closest is the PR itself); a pre-migration discussion,
  omacom/omarchy#4706 ("Replacing Mako"), predates the Quickshell rewrite and
  is unrelated (a rejected notification-center feature request against the
  old Mako-based setup).
- **Action-button support — confirms the local finding.** No evidence found
  of any multi-button action UI, upstream or in the wild. The single
  click-action path was recently *hardened*, not extended: PR #7926 "Run
  notification click actions as safe argv" reworked the click handler from a
  free-form `bash -lc` string into a safe argv vector — security work on the
  existing one-action design, not a step toward multi-action buttons.

## Answer summary (for the issue comment)

- **(a)** The `shell` IPC target (`ping`/`summon`/`toggle`/`call`/`listPlugins`/etc.,
  over Quickshell's own `qs ipc` mechanism, not D-Bus) is documented in-tree and
  empirically works for any session process with no special registration. But
  *showing new custom UI* requires being a discovered plugin (first-party, or
  dropped into `~/.config/omarchy/plugins/<id>/` and enabled) — there's a
  documented, non-interactive (`--yes`) install path explicitly aimed at
  scripts/agents, but it's a one-time setup step, not a zero-install call.
- **(b)** Yes: `omarchy-shell`'s own `omarchy.notifications` service plugin
  registers `org.freedesktop.Notifications` on the session bus (no
  mako/dunst/swaync) and advertises the `actions` capability at the protocol
  level, but the shipped notification card only wires up a single `"default"`
  click action plus dismiss — no multi-button action UI is actually rendered.
  Not viable for a real Deny/Allow-once/Allow-session choice as shipped.
- **(c)** A standalone wlr-layer-shell popup is the right fallback — Hyprland
  supports the protocol natively, and it's exactly what Omarchy's own polkit
  agent dialog is built on (`PanelWindow` + `WlrLayershell.layer: Overlay` +
  `keyboardFocus: Exclusive`, via Quickshell's `Quickshell.Wayland` QML
  module). `gtk4-layer-shell` is already installed on this machine as an
  independent, non-Quickshell option; a standalone Quickshell instance is the
  same-toolkit alternative.
