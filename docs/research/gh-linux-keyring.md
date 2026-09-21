# Does `gh` use the Linux Secret Service keyring, and when does it fall back?

**Summary:** Yes. Since April 2023, `gh auth login` (and `gh auth refresh`/`switch`/`logout`) default to "secure storage": the OAuth/PAT token is written via `github.com/zalando/go-keyring` (pinned at `v0.2.8` in `cli/cli`'s `go.mod`), which on Linux talks to the freedesktop **Secret Service D-Bus API** (`org.freedesktop.secrets`, normally backed by gnome-keyring or KWallet) over the **session** D-Bus bus. If that call fails for any reason — no session D-Bus bus (headless box, bare SSH session, container without a keyring daemon), no Secret Service provider registered, a locked/unavailable collection, etc. — `gh` silently falls back to writing the token in **plain text** to `hosts.yml` under `oauth_token`, and prints `! Authentication credentials saved in plain text` to stderr. Independent of all of this, `GH_TOKEN`/`GITHUB_TOKEN` (and their `*_ENTERPRISE_TOKEN` siblings) are checked **before** either the plaintext config or the keyring are ever touched, and take precedence over anything stored on disk.

---

## 1. Does `gh` use a Go keyring library to hit the Linux Secret Service, and under what conditions?

**Finding:** Yes. `cli/cli` depends directly on `github.com/zalando/go-keyring v0.2.8` (see `go.mod`), wrapped by a small internal package `internal/keyring` that just adds a 60-second timeout around `Set`/`Get`/`Delete`. `internal/config/config.go`'s `AuthConfig.Login`, `TokenFromKeyring`, `TokenFromKeyringForUser`, `SwitchUser`, `Logout`, and `activateUser` all call through this wrapper using a per-host service name `"gh:" + hostname`.

`zalando/go-keyring`'s Linux/BSD build (`keyring_unix.go`, build-tagged for `linux`, and cgo-gated `dragonfly`/`freebsd`) implements the `Keyring` interface via a `secretServiceProvider` that talks to D-Bus's Secret Service API (package `zalando/go-keyring/secret_service`). Its `NewSecretService()` does exactly one thing to establish connectivity: `dbus.SessionBus()`. So the precondition for `gh` to actually reach a Secret Service backend on Linux is: a reachable **session** D-Bus bus (i.e. `DBUS_SESSION_BUS_ADDRESS` resolvable / a running `dbus-daemon --session` or systemd user D-Bus), **and** something registered as `org.freedesktop.secrets` on that bus (gnome-keyring-daemon, KWallet's `ksecretservice` module, kwallet's Secret Service bridge, `keepassxc` with its Secret Service integration, etc.), **and** the login/default collection able to be unlocked (`svc.Unlock(collection.Path())` is called before every read/write).

Secure storage is the CLI's **default** since 2023-04-04 — confirmed directly in a code comment:

```go
// pkg/cmd/auth/login/login.go, lines 157-160
// secure storage became the default on 2023/4/04; this flag is left as a no-op for backwards compatibility
var secureStorage bool
cmd.Flags().BoolVar(&secureStorage, "secure-storage", false, "Save authentication credentials in secure credential store")
_ = cmd.Flags().MarkHidden("secure-storage")
```

The actual login call:

```go
// pkg/cmd/auth/login/login.go, line 216 (and 242 for the interactive/web flow)
_, loginErr := authCfg.Login(hostname, username, opts.Token, opts.GitProtocol, !opts.InsecureStorage)
```

`opts.InsecureStorage` defaults to `false`, so `secureStorage` is `true` unless the user explicitly passes `--insecure-storage`. `AuthConfig.Login` then tries the keyring first:

```go
// internal/config/config.go, lines 419-435
func (c *AuthConfig) Login(hostname, username, token, gitProtocol string, secureStorage bool) (bool, error) {
	var setErr error
	if secureStorage {
		setErr = keyring.Set(keyringServiceName(hostname), username, token)
		if setErr == nil {
			_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username, oauthTokenKey})
		}
	}
	insecureStorageUsed := false
	if !secureStorage || setErr != nil {
		c.cfg.Set([]string{hostsKey, hostname, usersKey, username, oauthTokenKey}, token)
		insecureStorageUsed = true
	}
	...
```

The CLI's own docs corroborate this in the `gh auth login` help text (`pkg/cmd/auth/login/login.go`, lines 68-72):

> "The default authentication mode is a web-based browser flow. After completion, an authentication token will be stored securely in the system credential store. If a credential store is not found or there is an issue using it gh will fallback to writing the token to a plain text file. See `gh auth status` for its stored location."

There is no Linux-equivalent of `docs/macos-keyring.md` in the repo (I searched via GitHub code search for `keyring` across `cli/cli` and only `docs/macos-keyring.md` exists — it documents the macOS `/usr/bin/security`-exec approach specifically). The Linux/Secret-Service path is not separately documented; the above is reconstructed from the shared `internal/config/config.go` logic (which is OS-agnostic) plus `zalando/go-keyring`'s Linux implementation.

Every command that needs a token also *reads* from the keyring (not just `login`): `AuthConfig.ActiveToken` tries `TokenFromKeyringForUser`/`TokenFromKeyring` whenever no env var or plaintext token was found (see section 2).

---

## 2. What does `gh` fall back to when no keyring/D-Bus session is available?

**Finding:** Plain text, in `hosts.yml`, under an `oauth_token` field — exactly as the help text above states. This is not a refusal; it's a silent, automatic fallback (with a one-line stderr warning).

Mechanically: `keyring.Set` (and therefore `zalando/go-keyring`'s `secretServiceProvider.Set`) returns a non-nil `error` when `dbus.SessionBus()` fails (no D-Bus session available) or any subsequent D-Bus call fails (no Secret Service registered, unlock refused, etc.):

```go
// github.com/zalando/go-keyring secret_service/secret_service.go, lines 56-61 (tag v0.2.8)
func NewSecretService() (*SecretService, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	...
```

Back in `cli/cli`, that propagates as `setErr != nil`, which flips the `Login` function into the `!secureStorage || setErr != nil` branch shown above, writing the token to the config store instead of the keyring and setting `insecureStorageUsed = true`. The caller then prints a warning:

```go
// pkg/cmd/auth/shared/login_flow.go, lines 204-210
insecureStorageUsed, err := cfg.Login(hostname, username, authToken, gitProtocol, opts.SecureStorage)
if err != nil {
	return err
}
if insecureStorageUsed {
	fmt.Fprintf(opts.IO.ErrOut, "%s Authentication credentials saved in plain text\n", cs.Yellow("!"))
}
```

The plaintext value is stored as `hosts: <hostname>: oauth_token: <token>` (and per-user under `users: <username>: oauth_token`) via the `oauthTokenKey = "oauth_token"` constant (`internal/config/config.go`, line 32) and written to `hosts.yml`. `gh auth status` explicitly reconstructs the file path when the token source is `"oauth_token"`:

```go
// pkg/cmd/auth/status/status.go, lines 375-381
if tokenSource == "oauth_token" {
	// The go-gh function TokenForHost returns this value as source for tokens read from the
	// config file, but we want the file path instead. This attempts to reconstruct it.
	tokenSource = filepath.Join(config.ConfigDir(), "hosts.yml")
}
```

`config.ConfigDir()` (in `cli/go-gh`'s `pkg/config/config.go`) resolves to `$GH_CONFIG_DIR`, else `$XDG_CONFIG_HOME/gh`, else `~/.config/gh` (with an `AppData` branch for Windows) — so on a typical Linux box that's `~/.config/gh/hosts.yml`, matching the file `hostsConfigFile()` returns (`filepath.Join(ConfigDir(), "hosts.yml")`).

There is also an explicit opt-in flag, `--insecure-storage`, that skips the keyring attempt entirely and always writes plaintext (`pkg/cmd/auth/login/login.go`, line 162). `gh` does **not** refuse to store credentials when the keyring is unavailable — it degrades gracefully to the plaintext file, by design.

(I did not find a documented list of the exact D-Bus/Secret Service error conditions that trigger the fallback — `zalando/go-keyring` just surfaces whatever error `dbus.SessionBus()` or the subsequent D-Bus calls return, and `gh` treats any non-nil error the same way, as a trigger to fall back. I did not find a primary source enumerating "these specific D-Bus error codes are exempted" — none are; any error triggers the fallback.)

---

## 3. Do `GH_TOKEN`/`GITHUB_TOKEN` bypass keyring/hosts.yml lookup entirely?

**Finding:** Yes. They are checked first, before the plaintext `oauth_token` config value and before the keyring is ever queried, and once set they take precedence over anything on disk.

`AuthConfig.ActiveToken` (the single function all token resolution funnels through) checks env-or-config first, and only falls through to the keyring if that returned empty:

```go
// internal/config/config.go, lines 254-280
func (c *AuthConfig) ActiveToken(hostname string) (string, string) {
	if c.tokenOverride != nil {
		return c.tokenOverride(hostname)
	}
	token, source := ghauth.TokenFromEnvOrConfig(hostname)
	if token == "" {
		var user string
		var err error
		if user, err = c.ActiveUser(hostname); err == nil {
			token, err = c.TokenFromKeyringForUser(hostname, user)
		}
		if err != nil {
			token, err = c.TokenFromKeyring(hostname)
		}
		if err == nil {
			source = "keyring"
		}
	}
	return token, source
}
```

`ghauth.TokenFromEnvOrConfig` (in `cli/go-gh`, `pkg/auth/auth.go`) itself checks env vars before ever reading the plaintext config value:

```go
// github.com/cli/go-gh v2 pkg/auth/auth.go, lines 63-100
func tokenForHost(cfg *config.Config, host string) (string, string) {
	normalizedHost := NormalizeHostname(host)
	if normalizedHost == github || IsTenancy(normalizedHost) || normalizedHost == localhost {
		if token := os.Getenv(ghToken); token != "" {          // GH_TOKEN
			return token, ghToken
		}
		if token := os.Getenv(githubToken); token != "" {      // GITHUB_TOKEN
			return token, githubToken
		}
	} else {
		if token := os.Getenv(ghEnterpriseToken); token != "" {      // GH_ENTERPRISE_TOKEN
			return token, ghEnterpriseToken
		}
		if token := os.Getenv(githubEnterpriseToken); token != "" {  // GITHUB_ENTERPRISE_TOKEN
			return token, githubEnterpriseToken
		}
	}
	...
	token, err := cfg.Get([]string{hostsKey, normalizedHost, oauthToken}) // plaintext hosts.yml
	...
}
```

So the true resolution order for a given host is: **(1) `GH_TOKEN`/`GITHUB_TOKEN` env var → (2) plaintext `oauth_token` in `hosts.yml` → (3) Secret Service keyring**. Env vars win outright; the keyring is the *last* resort checked, not a peer of env vars.

This is also documented in `gh`'s own built-in help (source of `gh help environment`):

```go
// pkg/cmd/root/help_topic.go, lines 45-47
%[1]sGH_TOKEN%[1]s, %[1]sGITHUB_TOKEN%[1]s (in order of precedence): an authentication token that will be used when
a command targets either %[1]sgithub.com%[1]s or a subdomain of %[1]sghe.com%[1]s. Setting this avoids being prompted to
authenticate and takes precedence over previously stored credentials.
```

Additionally, when an env var is active, `gh auth login` for that host is explicitly blocked (it won't let you create a second, stored credential that would be shadowed by the environment variable):

```go
// pkg/cmd/auth/shared/writeable.go
func AuthTokenWriteable(authCfg gh.AuthConfig, hostname string) (string, bool) {
	token, src := authCfg.ActiveToken(hostname)
	return src, (token == "" || !strings.HasSuffix(src, "_TOKEN"))
}
```
```go
// pkg/cmd/auth/login/login.go, lines 190-194
if src, writeable := shared.AuthTokenWriteable(authCfg, hostname); !writeable {
	fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
	fmt.Fprint(opts.IO.ErrOut, "To have GitHub CLI store credentials instead, first clear the value from the environment.\n")
	return cmdutil.SilentError
}
```

---

## Implications for CmdWarden-Omarchy Compat Mode

- **Don't build a parallel credential store for `gh`.** `gh` already does the "try Secret Service, fall back to plaintext" dance itself. Compat Mode should not attempt to intercept, pre-populate, or migrate `gh`'s keyring/`hosts.yml` entries — it would duplicate logic that's already OS-correct and change out from under `gh`'s own fallback/warning behavior.
- **`GH_TOKEN`/`GITHUB_TOKEN` are the highest-precedence override, above whatever `gh` has stored.** If Compat Mode ever needs to hand `gh` a scoped/ephemeral credential (e.g., for a sandboxed/gated invocation), setting `GH_TOKEN` in the child process's environment is a clean, well-supported way to do it — it bypasses both the keyring and the plaintext file with no ambiguity, and doesn't require touching `~/.config/gh/hosts.yml` or D-Bus at all.
- **A missing/locked Secret Service on Omarchy/Hyprland (e.g., no `gnome-keyring`/`kwallet` daemon running, or a session started without one) means `gh auth login` will silently degrade to plaintext `~/.config/gh/hosts.yml`.** CmdWarden-Omarchy's threat model/docs should call this out explicitly if it cares about "is the GitHub token encrypted at rest" — that depends on the desktop's Secret Service setup, not on anything `gh` or CmdWarden controls, and `gh` will not warn beyond the one-line stderr message at the moment of `login`/`refresh`/`switch`.
- **Detecting "is `gh`'s token currently coming from the keyring or from plaintext" is possible without touching gh's internals**: `gh auth status` surfaces the token source, and reconstructs the `hosts.yml` path specifically when the source is `oauth_token` (plaintext). Compat Mode can shell out to `gh auth status` (or parse `hosts.yml` presence) rather than reimplementing `zalando/go-keyring` lookups.
- **If Compat Mode ever wraps `gh auth login`/`logout`/`switch`/`refresh`**, be aware these are the only commands that *write* to the keyring; all other `gh` commands only *read* the active token via `ActiveToken`. Wrapping/gating should treat those four subcommands specially (they mutate credential storage state), while treating everything else as a read-only credential consumer.

---

## Sources

- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/internal/keyring/keyring.go — `cli/cli` source, thin timeout wrapper around `zalando/go-keyring` (`Set`/`Get`/`Delete`). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/internal/config/config.go — `cli/cli` source, `AuthConfig` implementation: `ActiveToken` (lines 254-280), `TokenFromKeyring`/`TokenFromKeyringForUser` (317-335), `Login` (416-453), `keyringServiceName` (577-579), `oauthTokenKey` constant (line 32). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/internal/gh/gh.go — `cli/cli` source, `AuthConfig`/`Config` domain interfaces and doc comments (e.g. `Login` doc: "will first try to store the auth token in encrypted storage and will fall back to the general insecure configuration"). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/pkg/cmd/auth/login/login.go — `cli/cli` source, `gh auth login` command: help text (lines 62-96), `--secure-storage`/`--insecure-storage` flags (157-163), `AuthTokenWriteable` gate (190-194), call into `authCfg.Login` (216, 242). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/pkg/cmd/auth/shared/login_flow.go — `cli/cli` source, prints `"Authentication credentials saved in plain text"` warning when `insecureStorageUsed` (lines 204-210). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/pkg/cmd/auth/shared/writeable.go — `cli/cli` source, `AuthTokenWriteable`: refuses `gh auth login` when the active token source is an env var. Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/pkg/cmd/auth/status/status.go — `cli/cli` source, `gh auth status`; reconstructs `hosts.yml` path when token source is `oauth_token` (lines 375-381). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/pkg/cmd/root/help_topic.go — `cli/cli` source, built-in text for `gh help environment`, defining `GH_TOKEN`/`GITHUB_TOKEN` precedence (lines 43-56). Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/docs/macos-keyring.md — `cli/cli` official docs (macOS-specific; no Linux equivalent exists in the repo, confirmed via GitHub code search for "keyring" across `cli/cli`). Corroborates that `zalando/go-keyring` is the storage backend and lists which commands touch the keyring. Accessed 2026-09-20.
- https://github.com/cli/cli/blob/0cf1092493af067646fc5f3db9421c6a6ec9c938/go.mod — `cli/cli` dependency manifest; pins `github.com/zalando/go-keyring v0.2.8` (line 59). Accessed 2026-09-20.
- https://github.com/cli/go-gh/blob/37aa5bbaf1a591aa134913efeb3775f0235dc5ab/pkg/auth/auth.go — `cli/go-gh` (v2) source, `TokenForHost`/`TokenFromEnvOrConfig`/`tokenForHost`: env-var-before-config resolution order (lines 16-100). Accessed 2026-09-20.
- https://github.com/cli/go-gh/blob/37aa5bbaf1a591aa134913efeb3775f0235dc5ab/pkg/config/config.go — `cli/go-gh` source, `ConfigDir()`/`hostsConfigFile()` resolving `~/.config/gh/hosts.yml` (or `$GH_CONFIG_DIR`/`$XDG_CONFIG_HOME`) (lines 228-252). Accessed 2026-09-20.
- https://github.com/zalando/go-keyring/blob/v0.2.8/keyring.go — `zalando/go-keyring` library source, top-level `Keyring` interface and the `provider`/`fallbackServiceProvider` dispatch. Accessed 2026-09-20.
- https://github.com/zalando/go-keyring/blob/v0.2.8/keyring_unix.go — `zalando/go-keyring` library source, Linux/BSD build tag (`linux`, cgo-gated `dragonfly`/`freebsd`/`netbsd`/`openbsd`) and `secretServiceProvider` implementation (Set/Get/Delete over D-Bus Secret Service). Accessed 2026-09-20.
- https://github.com/zalando/go-keyring/blob/v0.2.8/keyring_fallback.go — `zalando/go-keyring` library source, `ErrUnsupportedPlatform` fallback provider used only for platforms with no build-tag match (not normally reached on Linux, since `keyring_unix.go` covers `linux` unconditionally). Accessed 2026-09-20.
- https://github.com/zalando/go-keyring/blob/v0.2.8/secret_service/secret_service.go — `zalando/go-keyring` library source, `NewSecretService()` calling `dbus.SessionBus()` (lines 56-67); this is the exact point where "no session D-Bus" surfaces as an error that triggers `gh`'s plaintext fallback. Accessed 2026-09-20.
- https://cli.github.com/manual/gh_auth_login — official `gh` manual page (generated from the same help text cited above from `login.go`); states default secure storage and plaintext fallback behavior in prose. Accessed 2026-09-20 via fetch.
- https://cli.github.com/manual/gh_help_environment — official `gh` manual page (generated from `help_topic.go`); documents `GH_TOKEN`/`GITHUB_TOKEN` precedence order and that they "take precedence over previously stored credentials." Accessed 2026-09-20 via fetch.
