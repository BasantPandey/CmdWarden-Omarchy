# CLI reference

`cw` (alias `cmdwarden`) is the only binary you invoke directly.
`cmdwarden-agent` runs in the background, lazy-started by systemd — you
normally never invoke it yourself.

## Agent lifecycle

```sh
cw doctor                # lazy-starts the agent if needed, reports health
                          # (Session Agent reachability, Shim Pin Drift)
cw agent status           # is the agent running and healthy right now?
cw agent stop             # stop the running agent
cw agent install           # write + enable the systemd --user socket unit
cw agent uninstall         # stop, disable, and remove the unit files
```

## Identity

```sh
cw whoami                 # print this shell's resolved Identity Key
cw whoami -v               # ...plus channel/tool/path/hash
```

## Policy

```sh
cw policy enroll --kind ai-harness|terminal [--key <identity-key>]
                          # enroll a Launcher (defaults to the caller,
                          # resolved the same way `cw whoami` does).
                          # ai-harness -> Read, terminal -> Trusted.
cw policy list             # list every enrolled Launcher and its level
cw policy set <identity-key> <Deny|Read|Trusted|Full>
cw policy unenroll <identity-key>
cw policy check [--identity <key>] --class read|write|secret-reveal|unknown
                          # dry-run the policy decision without running
                          # anything gated
```

## Vault

```sh
cw vault save <name>       # prompts (hidden input) or reads stdin if piped
cw vault delete <name>
cw vault import gh [--hostname github.com]
                          # import gh's currently active token
cw vault exec --secret <name> --env <VAR> -- <command> [args...]
                          # release a secret into exactly one child
                          # process's environment; never printed or logged
```

## Shim

```sh
cw shim install --tool <name> [--path <explicit-binary-path>]
                          # installs Occupied (mise) or Path (pacman) Shim,
                          # branching automatically by Provenance Channel
cw shim uninstall --tool <name>
cw shim list
```

## Harden (gh only, in this spike vertical)

```sh
cw harden gh [--hostname github.com]
                          # resolve gh's PATH, import its token, install
                          # the Shim, record the pin. Safe to re-run at
                          # any time (removes a stale pin first).
cw unharden gh [--hostname github.com]
                          # remove the Shim, restore the real binary,
                          # delete the vault entry
```

## Approval Gate

```sh
cw gate test [--identity <key>] --tool <name> --command "<display line>" \
             --class read|write|secret-reveal|unknown
                          # trigger a real Approval Gate popup with the
                          # caller's real identity/policy and fake command
                          # data — for testing the gate itself
```

`cw gate respond` also exists but is hidden: it's what the gate UI's own
buttons invoke, never meant for direct use.

## Audit

```sh
cw audit                  # dump the whole gate-decision log (NDJSON)
cw audit --follow          # ...and keep tailing it
cw audit prune             # drop rows older than the retention window (30d)
```

## Everything else

```sh
cw version
cw help
cw <command> --help
```
