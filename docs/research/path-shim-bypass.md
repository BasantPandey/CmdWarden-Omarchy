# Which calls bypass a PATH-prepended `gh` shim?

**Summary:** A pacman Path Shim never replaces `/usr/bin/gh` (or whichever path pacman owns). It only prepends `~/.local/share/cmdwarden/shims` on `PATH` for processes that source the bootstrap or inherit that `PATH`. NAV's cplt security model names the same limit for its own `gh`/`git` PATH shims: an absolute path, `env`, a reset `PATH`, a shell escape, or a call from another runtime can run the real binary and never enter the shim. An Occupied Shim for a mise install closes the forms that still execute the original file, because that file is now the shim script. It does not close a call that executes a different `gh`, including the renamed `<path>.cmdwarden-real` file.

This note does not propose a build.

---

## What this repo installs

`Install` branches on provenance (`internal/shim/install.go`):

- **Occupied Shim** (`installOccupied`, mise). `filepath.EvalSymlinks` resolves the target. The file is renamed to `<resolved>.cmdwarden-real`. The shim script is written at the old path, mode `0755`. No `PATH` edit.
- **Path Shim** (`installPath`, pacman). The real binary is not moved. `RealBinaryPath` stays equal to `OriginalPath`. A script named `gh` is written under `ShimDir()` (`$XDG_DATA_HOME/cmdwarden/shims`, default `~/.local/share/cmdwarden/shims`). `ensurePathBootstrap` then prepends that directory.

Both scripts are the same text (`internal/shim/script.go`): `exec cw shim-exec --tool NAME --real PATH -- "$@"`. The real path is baked in at install time. A caller that never runs the script never reaches `cw shim-exec`. The baked path does not matter.

The prepend is not a PAM or `/etc` change. `pathshim.go` writes four source blocks plus one systemd user environment file:

| Hook | File | When it runs |
|---|---|---|
| login shell | `~/.profile` | bash login shells, after `/etc/profile` (`Bash Startup Files`, "Invoked as an interactive login shell") |
| interactive bash | `~/.bashrc` | interactive non-login bash. Not a non-interactive shell |
| interactive zsh | `~/.zshrc` | interactive zsh only. Not `zsh -c` |
| uwsm session | `~/.config/uwsm/env.d/50-cmdwarden` | Hyprland/uwsm session start |
| systemd user | `~/.config/environment.d/50-cmdwarden-shim.conf` | a **new** `systemd --user` session. The file sets `BASH_ENV` to `shim-env-bootstrap.sh`. It does not set `PATH` |

`docs/architecture.md` (Shim) and `README.md` (State and data locations) describe the same split. `pathshim.go` says `BASH_ENV` is the per-user stand-in for Omarchy's PAM `PATH` line, because `cw` has no root. The same comment says `environment.d` does not affect shells that are already running, and that `~/.bashrc` is not read for a non-interactive non-login shell.

`cw doctor` can report Pin Drift after the fact (`internal/shim/drift.go`). For a Path Shim, drift is "something else now wins earlier on `PATH`". That check does not stop the call.

The primary list of these invocation shapes is cplt's own security model, under "PATH-shim enforcement":

> The guards write `gh`/`git` shims into a scratch `bin/` dir and prepend it to `PATH` ... The real `/usr/bin/git` and `/usr/bin/gh` are untouched, so any agent that invokes the binary by **absolute path** (`/usr/bin/git push`), **escapes the alias** (`\git`, `env git`), **runs the real binary under another name** (`ln -s "$(command -v git)" g && PATH=$PWD:$PATH g push`), **resets `PATH`**, or **shells out from another runtime** (`subprocess.run(["/usr/bin/git", "push"])`) bypasses the guard entirely.

A second paragraph, "The PATH shims", states the same boundary for a directory prepended on `PATH`: an absolute path, a reordered `PATH`, or a launcher that never ran a shell does not hit the shim. ([`SECURITY.md` at `a76955d`](https://github.com/navikt/cplt/blob/a76955d532b16f53e21e39012ee923b6f51eddce/SECURITY.md), accessed 2026-09-27.)

---

## 1. Absolute path

**Finding:** Open for a pacman Path Shim. Closed for a mise Occupied Shim, but only for the original file path.

POSIX `execvp()` uses the pathname as given when it contains a slash. It does not search `PATH`. ([POSIX `exec`](https://pubs.opengroup.org/onlinepubs/9699919799/functions/exec.html), "If the file argument to any of these functions contains a slash, the file argument shall be used as the pathname".) cplt's report uses `/usr/bin/git push` as the example, and says the real `/usr/bin/gh` is untouched.

`installPath` matches that: the pacman file stays in place (`internal/shim/install.go`). `/usr/bin/gh pr create` runs pacman `gh`. The shim directory is never consulted.

`installOccupied` does the opposite for the mise file. The path that used to be the binary is now the shim script. An absolute exec of that path, or of a symlink that still points at it, runs the shim. `docs/architecture.md` states this as "every resolution path that used to reach the real binary now reaches the Shim".

Two absolute paths stay open after an Occupied install:

- `<resolved>.cmdwarden-real`, the name `installOccupied` gives the moved binary. That path did not exist before install. It is a normal executable file.
- Any other `gh`, such as pacman `/usr/bin/gh`, if one is also installed. Occupied mode only replaces the mise file.

A later mise upgrade that repoints the stable `current` symlink at a new binary is a separate hole. `drift_test.go` (`TestDetectDriftFlagsNewVersionAfterUpgrade`) and `docs/architecture.md` already describe it. It is not one of the five call shapes in the cplt paragraph.

---

## 2. `env`

**Finding:** `env gh` does not skip a `PATH` search. It bypasses a Path Shim only when `env` replaces `PATH` or drops the inherited environment. An Occupied Shim still runs if the `PATH` that `env` uses still contains the occupied directory.

cplt lists `env git` next to `\git`, under "escapes the alias", in the PATH-shim paragraph cited above. That pair is how you skip a shell alias or function. It is not, by itself, how you skip a directory that is already first on `PATH`.

POSIX `env` searches `PATH` for the utility. If a `name=value` operand sets `PATH`, that value is the search path. `env -i` ignores the inherited environment. ([POSIX `env`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/env.html), ENVIRONMENT VARIABLES, `PATH`, and the `-i` option.) Historical `env` calls `execvp()` / `execlp()`, so it does not see shell functions or aliases (same page, APPLICATION USAGE).

Applied here:

- `env gh`, with the session `PATH` unchanged, finds the Path Shim when the bootstrap has already prepended `shims/`. It finds the Occupied Shim when `PATH` contains the mise directory, in any position. The file there is the script.
- `env PATH=/usr/bin:/bin gh` and `env -i PATH=/usr/bin:/bin gh` are a reset `PATH` done by `env`. They are open for a Path Shim: the first `gh` is the pacman binary. They are open for an Occupied Shim only when that new `PATH` names a different `gh`. If the new `PATH` still contains the occupied directory, `execvp()` still runs the script.

---

## 3. A reset `PATH`

**Finding:** Open for a pacman Path Shim. Closed for a mise Occupied Shim when the new `PATH` still contains the occupied directory. Open for Occupied when the new `PATH` resolves a different `gh`.

cplt names this twice: "resets `PATH`" in the PATH-shim enforcement paragraph, and "a reordered `PATH`" in "The PATH shims". The alias-escape example on the same line also builds a new `PATH`: `PATH=$PWD:$PATH g push`.

A Path Shim wins only while its directory is the first `gh` on `PATH`. `PATH=/usr/bin:/bin gh`, or any prepend of another directory that contains `gh`, runs the other file. `drift.go` treats that as Pin Drift for `ModePath` ("something else now wins earlier on `PATH`"). The call is already ungated when `doctor` runs.

An Occupied Shim does not care about order. `installOccupied` replaced the file. A reset `PATH` that still lists that directory runs the script. A reset that omits that directory and lists another `gh` runs the other file. A reset that omits every `gh` fails the lookup. That is a miss, not a second copy of the binary.

The bootstrap itself can fail to be present. `environment.d` is read for a new systemd user manager, not for an existing one (`pathshim.go`). A shell that started before install keeps its old `PATH` until it is started again. That is the same class as a `PATH` that was never prepended.

---

## 4. A shell escape

**Finding:** `\gh` and `command gh` do not bypass either shim while normal `PATH` search still sees it. `command -p`, a non-interactive `sh`, and a `zsh -c` that did not inherit the prepend, do bypass a Path Shim. `bash -c` does not, once `BASH_ENV` from a new user session points at the bootstrap. Occupied mode is unchanged by these spellings, because they still execute whatever file `PATH` names.

cplt's shell-alias paragraph says a rc-file alias does nothing for an absolute path, for `\copilot` or `command copilot`, or for a non-interactive shell, a script, or a launcher that never sources the rc file. Its PATH-shim paragraph says a prepended directory does reach `zsh -c`, scripts, and IDE-spawned agents **when their `PATH` starts with the shim directory**. A launcher that never ran a shell does not get that `PATH`.

Bash agrees, and the two mechanisms are different:

- Quoting any character of a word stops alias expansion for that word. ([Bash Aliases](https://www.gnu.org/software/bash/manual/html_node/Aliases.html).) `\gh` is still a `PATH` search.
- `command gh` ignores a function of that name, then searches `PATH`. `command -p` searches a default `PATH` "guaranteed to find all of the standard utilities", not the user's `PATH`. ([Bash Builtin Commands](https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html), `command`.) On a typical Arch layout that default `PATH` contains `/usr/bin/gh` and does not contain `~/.local/share/cmdwarden/shims` or a mise install dir. `command -p gh` is open for a Path Shim. It is open for an Occupied Shim when `/usr/bin/gh` exists. If the only `gh` is the mise file, `command -p` does not find it (exit 127). That is not a run of the real mise binary.

Non-interactive startup is the escape `pathshim.go` tries to close, and only for bash:

- Non-interactive bash reads `BASH_ENV` and does not read `~/.bashrc`. ([Bash Startup Files](https://www.gnu.org/software/bash/manual/html_node/Bash-Startup-Files.html), "Invoked non-interactively".) After a new `systemd --user` session, `50-cmdwarden-shim.conf` sets `BASH_ENV`, so `bash -c 'gh ...'` sources the bootstrap and the Path Shim runs.
- Bash invoked as `sh` does not do that. A non-interactive shell invoked with the name `sh` reads no startup files (same manual, "Invoked with name sh"). `sh -c 'gh ...'` keeps the `PATH` it inherited. If the parent never had the prepend, the Path Shim is skipped.
- `env -u BASH_ENV bash -c 'gh ...'` removes the hook for that process. Same result.
- zsh reads `~/.zshrc` only when the shell is interactive. ([Zsh files](https://zsh.sourceforge.io/Doc/Release/Files.html): `.zshenv` for every shell, then `.zshrc` only if interactive.) This repo writes `~/.zshrc`, not `~/.zshenv` (`bootstrapTargets` in `pathshim.go`). `zsh -c 'gh ...'` does not source the block. It hits the Path Shim only when the parent environment already has the directory on `PATH` (a login shell, an interactive shell, or the uwsm session). cplt's claim that a PATH shim reaches `zsh -c` is true for cplt because cplt puts the directory on `PATH` in the environment those processes inherit. It is not true here for a `zsh -c` whose environment never received the prepend. `environment.d` sets `BASH_ENV`, which zsh does not read.

None of these spellings move the mise file. An Occupied Shim still runs when the search returns that path.

---

## 5. A call from another runtime

**Finding:** Open for a pacman Path Shim when the runtime passes an absolute path, or a bare name plus a `PATH` that does not list the shim directory first. Closed for a mise Occupied Shim when the runtime executes the original path, or looks up the bare name on a `PATH` that contains the occupied directory.

cplt's example is `subprocess.run(["/usr/bin/git", "push"])`. The real binary is executed. The scratch `bin` shim is not.

Python's own rule matches that example. `subprocess.Popen` on POSIX follows `os.execvpe()`. A path with a directory separator is used as given. A bare name is searched on `PATH`. The `env` argument replaces the environment, so it can replace `PATH`. The docs say to prefer a fully qualified path, and they show `Popen(["/usr/bin/git", "commit", ...])`. ([Python `subprocess`](https://docs.python.org/3/library/subprocess.html#subprocess.Popen), accessed 2026-09-27.)

Go is the same. `exec.Command`: if `name` contains a path separator, that string is `Path`. Otherwise `LookPath` searches `PATH`. `LookPath`: if `file` contains a slash, it is tried directly and the default path is not consulted. ([`os/exec` on pkg.go.dev, Go 1.27.1](https://pkg.go.dev/os/exec#Command).) `exec.Command("/usr/bin/gh", "pr", "create")` never sees the Path Shim. `exec.Command("gh", ...)` sees it only when this process's `PATH` still has the shim directory first. `Cmd.Env` can replace that `PATH` for the child, but the lookup of a bare name happens in the parent, against the parent's `PATH`, before the child starts.

A runtime that inherited the prepended `PATH` from a shell or from uwsm does **not** bypass a Path Shim with a bare name. cplt's "launcher that never ran a shell" is the case that does: a systemd user service, an IDE spawn, or any `exec` whose environment was built without the bootstrap. `BASH_ENV` does not help. The runtime is not bash, so it never sources that file.

An Occupied Shim closes `exec.Command(originalPath)` and a bare `LookPath("gh")` that returns the occupied path. It does not close `exec.Command("/usr/bin/gh")` or `exec.Command(originalPath+".cmdwarden-real")`.

---

## Closed and open

**Occupied Shim (mise) already closes** these, for the file it replaced:

- An absolute path to that file (section 1).
- `env gh` and a reset `PATH`, when the `PATH` in use still contains the occupied directory (sections 2 and 3). Order does not matter.
- `\gh` and `command gh` when that search returns the occupied file (section 4).
- A bare-name call from Python, Go, or another runtime on such a `PATH`, and an absolute call to the original path (section 5).

**Occupied Shim leaves open:**

- An absolute call to `<path>.cmdwarden-real` (section 1).
- Any of the five shapes that resolve a different `gh`, including `/usr/bin/gh`, `command -p gh` when `/usr/bin/gh` exists, `env PATH=/usr/bin:/bin gh`, and `subprocess` / `os/exec` with that other path (sections 1 to 5).

**Path Shim (pacman) leaves open:**

- An absolute path to the pacman binary (section 1). cplt's report.
- `env` or an assignment that resets or reorders `PATH` so `shims/` is not first (sections 2 and 3). cplt's report.
- `command -p gh` (section 4). Bash manual.
- A shell that neither sources the bootstrap nor inherits the prepend: non-interactive `sh`, `zsh -c` in a clean environment, `env -u BASH_ENV bash -c`, and any shell still running from before the new `systemd --user` session (sections 3 and 4). Bash manual, zsh manual, `pathshim.go`.
- Another runtime that passes an absolute path, or a bare name with a `PATH` that does not list `shims/` first (section 5). cplt's report, Python docs, Go `os/exec`.

**Path Shim already covers**, and only when the prepend is really in that process:

- A bare `gh`, `env gh`, `\gh`, and `command gh` (sections 2 and 4).
- `bash -c 'gh ...'` after a new systemd user session, because `BASH_ENV` sources the bootstrap (section 4).
- A bare-name call from another runtime that inherited that `PATH` (section 5).

---

## Sources

- `internal/shim/install.go` - `installOccupied` renames the mise binary to `<path>.cmdwarden-real` and writes the script at the old path. `installPath` does not move the pacman binary. Read 2026-09-27.
- `internal/shim/pathshim.go` - bootstrap targets (`~/.profile`, `~/.bashrc`, `~/.zshrc`, uwsm `env.d`) and `~/.config/environment.d/50-cmdwarden-shim.conf` setting `BASH_ENV` only. Comment on non-interactive bash and on a new systemd user session. Read 2026-09-27.
- `internal/shim/script.go` - both shapes `exec` `cw shim-exec` with the tool name and real path baked in. Read 2026-09-27.
- `internal/shim/store.go` - package comment: Occupied replaces the file, Path Shim prepends `PATH`. Read 2026-09-27.
- `internal/shim/drift.go` - Path Shim drift is another path winning earlier on `PATH`. Detection only. Read 2026-09-27.
- `internal/shim/drift_test.go` - mise `current` symlink, and a version bump that leaves the shim behind. Read 2026-09-27.
- `docs/architecture.md` - Shim section. Occupied versus Path Shim, and the `BASH_ENV` hook. Read 2026-09-27.
- `README.md` - Path Shim directory, bootstrap script, and the files `cw shim uninstall` clears. Read 2026-09-27.
- https://github.com/navikt/cplt/blob/a76955d532b16f53e21e39012ee923b6f51eddce/SECURITY.md - cplt security model. "PATH-shim enforcement" (absolute path, `\git`, `env git`, reset `PATH`, other runtime) and "The PATH shims" (absolute path, reordered `PATH`, launcher that never ran a shell). Accessed 2026-09-27.
- https://pubs.opengroup.org/onlinepubs/9699919799/functions/exec.html - POSIX `execvp`: a pathname that contains a slash is not searched on `PATH`. Accessed 2026-09-27.
- https://pubs.opengroup.org/onlinepubs/9699919799/utilities/env.html - POSIX `env`: `PATH` search, `PATH` as a `name=value` operand, `-i`. Accessed 2026-09-27.
- https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html - Bash `command`, including `-p` and the default `PATH`. Accessed 2026-09-27.
- https://www.gnu.org/software/bash/manual/html_node/Aliases.html - quoting inhibits alias expansion. Accessed 2026-09-27.
- https://www.gnu.org/software/bash/manual/html_node/Bash-Startup-Files.html - `BASH_ENV` for non-interactive bash; `sh` reads no file in that case; `~/.bashrc` is for interactive non-login bash. Accessed 2026-09-27.
- https://zsh.sourceforge.io/Doc/Release/Files.html - `.zshrc` is interactive only. Accessed 2026-09-27.
- https://docs.python.org/3/library/subprocess.html#subprocess.Popen - absolute path versus `PATH` search; `env` replaces the environment. Accessed 2026-09-27.
- https://pkg.go.dev/os/exec#Command - Go `Command` / `LookPath`: a name that contains a slash is not a `PATH` search. Go 1.27.1 docs. Accessed 2026-09-27.
