# Which published failures from 2025 and 2026 does this local `gh` gate already stop?

**Summary:** The gate stops a failure only when the harmful step is a local `gh` process that the Shim actually runs, and only when policy does not auto-allow that class. On the default Read enrollment, that already covers `gh auth token` (the s1ngularity steal), `gh attestation` and `gh release verify` / `gh release verify-asset` (the 2026 token leak to TUF hosts), `gh codespace jupyter`, and other commands the classifier marks secret-reveal, write, or unknown. It does not stop the same incidents when the secret is read from a file, from the environment, or sent with a direct call to `api.github.com`. It does not stop GitHub Actions compromises, because those commands never run on this machine. It also does not stop several 2026 `gh` bugs whose commands the table marks read, so Read auto-allows them. Git's own 2025 advisories are a separate bucket. No git advisory on `git/git` was published in 2026 as of this note.

The product intercepts local `gh` on Linux. It does not patch `gh`, Claude Code, Cursor, or CI. A denied call never reaches the real binary. An allowed call still runs that binary, bugs included. `Full` auto-allows every class, including secret-reveal and unknown.

---

## How a call is allowed or refused

`cw harden gh` installs a Shim. The script is `exec cw shim-exec --tool gh --real PATH -- "$@"`. `shimexec` classifies the argv, asks policy, and execs the real binary only after an allow. An unenrolled Launcher is Deny, and Deny never prompts. Read auto-allows only `read`. Trusted also auto-allows `write`. `Full` auto-allows `read`, `write`, `secret-reveal`, and `unknown`. Anything else prompts, and the prompt fails closed. Secret-reveal is never offered "Allow for Session".

`gh auth token`, `gh auth git-credential get`, and `gh api` are secret-reveal. `gh auth status` is read, unless `--show-token` is present, in which case it is secret-reveal. `gh run view` and `gh codespace ports` are read, so `gh codespace ports forward` is read too (longest matching path wins). `gh gist view`, `gh pr diff`, and `gh release download` are read. `gh attestation`, `gh release verify`, `gh release verify-asset`, `gh codespace jupyter`, `gh codespace logs`, `gh skills`, and `gh agent-task` are not in the table, so they are unknown.

On an allow, the Shim injects the vault token as `GH_TOKEN` in that child only, and it removes any ambient `GH_TOKEN` first. On a deny, it does not exec `gh` at all.

A Path Shim (pacman `gh`) is a `PATH` prepend. An absolute path to the real binary skips it. An Occupied Shim (mise) sits on the real path, so a normal lookup cannot skip it.

---

## 1. The gate can stop it, because the action is a local `gh` call

These reports contain a step that is `gh` on the developer machine. Deny, or a refused prompt, stops that step before the real binary runs. They do not stop the rest of the same report. That rest is in section 2.

### 1.1 s1ngularity calls `gh auth token`

On 26 August 2025, malicious `nx` releases ran a `postinstall` script, `telemetry.js`. The Nx advisory is the vendor record: the script scans the file system, collects credentials, and posts them to a public GitHub repo whose name contains `s1ngularity-repository`. The advisory appendix quotes the script's file-search prompt. It does not quote the `gh` argv.

The preserved script, published in full by StepSecurity and matching that prompt, does this when `gh` is on `PATH`:

```javascript
spawnSync('gh', ['auth', 'token'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 5000 });
```

It keeps stdout only when the text starts with `gho_` or `ghp_`. It then creates the repo with a direct HTTPS `POST` to `api.github.com/user/repos`, not with `gh repo create`.

`gh auth token` is secret-reveal. An unenrolled Launcher (npm and node are not in the shell skip list, so npm is the Launcher) is a hard deny. A Read or Trusted Launcher gets a prompt, and secret-reveal cannot be granted for the session. Only `Full`, or an explicit Approve Once, lets the real `gh auth token` print the token. If the Shim denies the call, this copy of the script never receives `result.ghToken`, and the HTTPS upload that depends on that field does not run.

That is the only s1ngularity step this gate stops. Section 2.1 is the rest.

### 1.2 `gh attestation` and `gh release verify` send the token to the wrong host

[GHSA-8xvp-7hj6-mcj9](https://github.com/cli/cli/security/advisories/GHSA-8xvp-7hj6-mcj9) (CVE-2026-48501), published 27 May 2026, severity high. Affected: `gh` <= 2.92.0. Patched in 2.93.0.

`gh attestation`, `gh release verify`, and `gh release verify-asset` fetch TUF data through the same HTTP client that attaches the user's token. Host normalization treats `tuf-repo.github.com` as `github.com`, so the github.com token is sent to a GitHub Pages host. If `GH_ENTERPRISE_TOKEN` or `GITHUB_ENTERPRISE_TOKEN` is set, that token is sent to `tuf-repo-cdn.sigstore.dev` and `tmaproduction.blob.core.windows.net`.

None of those three commands is in the classifier table, so each is unknown. Read and Trusted prompt. Deny refuses. `Full` auto-allows, and an approved run still sends the header, because the gate does not patch `gh`. The vendor fix is to upgrade `gh` to 2.93.0 and revoke the token.

### 1.3 Attestation signer matching can be bypassed

[GHSA-mm27-mwq9-fr5g](https://github.com/cli/cli/security/advisories/GHSA-mm27-mwq9-fr5g) (CVE-2026-64655), published 31 July 2026, severity low. Affected: `gh` <= 2.96.0. Patched in 2.97.0.

`gh attestation verify --signer-repo` and `--signer-workflow` build a regular expression from the flag value and match it against the certificate SAN. The value is not escaped. GitHub allows `.` in a repository name, and `.` is a regex wildcard, so a lookalike signer can match a trusted name. `--repo` and `--owner` are not affected. The vendor example is `github/artifact.attestations-workflows` matching a check for `github/artifact-attestations-workflows`.

The command is unknown, so the same Deny / prompt rule as section 1.2 applies. An approved run still has the bug until `gh` is upgraded.

### 1.4 `gh attestation verify` can exit 0 when verification failed

[GHSA-fgw4-v983-mgp8](https://github.com/cli/cli/security/advisories/GHSA-fgw4-v983-mgp8) (CVE-2025-25204), published 14 February 2025, severity medium. `gh` 2.49.0 through 2.66.x. Patched in 2.67.0. First reported as [cli/cli#10418](https://github.com/cli/cli/issues/10418).

If an artifact has attestations, but none match `--predicate-type` (default `https://slsa.dev/provenance/v1`), the command prints a failure and still exits 0. A caller that trusts only the exit code treats that as success.

The local command is unknown, so the gate can refuse to run it. Refusal is not a fix. An allowed run still returns the wrong exit code. The same command on a CI runner does not pass through this Shim at all (section 2.3).

### 1.5 `gh codespace jupyter` can hand a crafted URL to VS Code

[GHSA-8cg3-r6g9-fpg2](https://github.com/cli/cli/security/advisories/GHSA-8cg3-r6g9-fpg2) (CVE-2026-59831), published 2 July 2026, severity medium. `gh` 2.10.0 through 2.95.x. Patched in 2.96.0.

`gh codespace jupyter` opens a JupyterLab URL supplied from inside the Codespace without checking that it is a loopback `http` or `https` address. A `vscode://` or `vscode-insiders://` URL can be handed to VS Code. This is a variant of the 2024 `gh codespace ssh` bug, which this note does not cover because that advisory was published in 2024.

`codespace jupyter` is not in the table, so it is unknown. The gate can refuse the call. An approved run still opens the URL until `gh` is upgraded.

### 1.6 Some terminal-escape commands are not classified as read

[GHSA-3m3g-3wcr-px46](https://github.com/cli/cli/security/advisories/GHSA-3m3g-3wcr-px46) (CVE-2026-64654), published 31 July 2026, severity medium. Patched in 2.97.0.

Several commands print attacker-controlled text without stripping terminal escape sequences. The vendor lists:

- `gh gist view` (only when the gist file is large enough that `gh` fetches the raw URL)
- `gh api` when the response is not JSON
- `gh pr diff`
- `gh release download --output -` (a download to a file is not affected)
- `gh codespace logs`
- `gh skills preview`
- `gh agent-task view` and `gh agent-task create`

`gh api` is secret-reveal, so Read and Trusted prompt and Deny refuses. `gh codespace logs`, `gh skills preview`, and `gh agent-task` are unknown, so the same rule applies. `gh gist view`, `gh pr diff`, and `gh release download` are read. Section 1.8 says why those three are not stopped on a normal Read enrollment.

### 1.7 A crafted path component can change the API URL

[GHSA-4fjg-2h4q-fwg3](https://github.com/cli/cli/security/advisories/GHSA-4fjg-2h4q-fwg3) (CVE-2026-64653), published 31 July 2026, severity low. Patched in 2.97.0.

Some REST URLs interpolate a path component without percent-encoding. A value that contains path metacharacters makes `gh` call a different endpoint than the user named. The classifier does not look at that value. It only looks at the subcommand. If that subcommand is write, secret-reveal, or unknown, the gate can refuse the process. If it is read, Read auto-allows it, and the bad URL still goes out. The gate does not fix encoding inside an allowed process.

### 1.8 Local `gh` calls the current table does not stop

These are still local `gh` calls. The Shim sees them. On the default Read enrollment they are auto-allowed, so the published failure still happens. Trusted and `Full` auto-allow them too, because every level that is not Deny auto-allows read. Only a Deny Launcher stops them, and Deny also stops ordinary reads.

- **Partial token in `gh auth status`.** [GHSA-cg6r-mpgc-h9mm](https://github.com/cli/cli/security/advisories/GHSA-cg6r-mpgc-h9mm) (CVE-2026-64652), published 31 July 2026, severity low. Patched in 2.97.0. Without `--show-token`, masking keeps only the text after the last underscore. Fine-grained PATs (`github_pat_*`) and GitHub App tokens (`ghs_*`, `ghu_*`), including an Actions `GITHUB_TOKEN`, can print a real prefix. Classic `gho_` and `ghp_` tokens are not affected. The command is read. `--show-token` upgrades it to secret-reveal, which does prompt. The buggy form does not use that flag, so the gate does not stop it.
- **Escape sequences in `gh run view --log`.** [GHSA-crc3-h8v6-qh57](https://github.com/cli/cli/security/advisories/GHSA-crc3-h8v6-qh57) (CVE-2026-45803), published 13 May 2026, severity low. Patched in 2.92.0. `gh run view --log` and `gh run view --log-failed` print Actions log lines with no escape filtering. `run view` is read, so the gate auto-allows it. The log content is produced in CI. The dangerous print is still a local `gh` call, and this gate lets that call through.
- **`gh codespace ports forward` listens on every interface.** [GHSA-vfhh-p7hm-pxfh](https://github.com/cli/cli/security/advisories/GHSA-vfhh-p7hm-pxfh) (CVE-2026-72924), published 20 August 2026, severity low. `gh` 2.28.0 through 2.97.x. Patched in 2.98.0. The local forward binds `*` rather than loopback, so a host that can route to the machine can open the port while the command runs. The longest table match is `codespace ports`, which is read. The gate auto-allows the vulnerable command.
- **The read half of CVE-2026-64654** (`gh gist view`, `gh pr diff`, `gh release download --output -`), same advisory as section 1.6. All three are read.

---

## 2. The gate cannot stop it, because the secret or the command never goes through `gh`

### 2.1 The rest of s1ngularity

The same `telemetry.js` also does all of the following, and none of it is a `gh` process:

- It copies `process.env` wholesale. A `GH_TOKEN` or `GITHUB_TOKEN` already in the environment is stolen with no `gh` call. The Shim removes ambient `GH_TOKEN` only inside the one allowed `gh` child. It does not clear the parent.
- It reads `~/.npmrc` via `fs.readFileSync` after `npm whoami`.
- It asks the `claude`, `gemini`, or `q` CLIs, with `--dangerously-skip-permissions`, `--yolo`, or `--trust-all-tools`, to write a file list to `/tmp/inventory.txt`. It then reads those files. The Nx advisory names `/tmp/inventory.txt` as the local indicator. This product does not intercept those CLIs.
- It appends `sudo shutdown -h 0` to `~/.bashrc` and `~/.zshrc`.
- After it has a token, it talks to `api.github.com` with Node's `https` module. A later use of an already stolen token does not pass through the Shim.
- The second wave, reported from 28 August 2025, used the stolen tokens from outside the machine to make private repos public and to fork them. That work is not a local `gh` call on the victim.

The first wave also started in GitHub Actions on `nrwl/nx`, not on the developer machine. A `pull_request_target` workflow interpolated the pull request title into a shell script, and that `GITHUB_TOKEN` was used to start `publish.yml` and send the npm publish token to a webhook. The vendor advisory records the workflow, the malicious commit `3905475cfd0e0ea670e20c6a9eaeb768169dc33d`, and the timeline. This gate does not run in GitHub Actions.

The Nx Console extension (VS Code 18.63.x through 18.65.x) installed `nx@latest` on startup during the same hours. That is an editor process, not a `gh` call. If that install then ran `telemetry.js`, only the `gh auth token` line in section 1.1 is gated.

### 2.2 Shai-Hulud reads files and calls the GitHub API

CISA's 23 September 2025 alert describes the first wave: a self-replicating npm worm, more than 500 packages. After install, the payload scans the machine for GitHub PATs and for cloud API keys. It uploads them by creating a public repository named Shai-Hulud through the GitHub `/user/repos` API, then publishes further npm packages with the stolen npm token. CISA does not describe a local `gh` process.

GitHub's own 23 December 2025 write-up of the same campaign says the malicious install code scavenges the local system for tokens and uses those tokens to propagate. It does not describe a local `gh` process either.

StepSecurity's write-up of the 24 November 2025 wave ("Sha1-Hulud: The Second Coming") says the payload takes the GitHub token from `GITHUB_TOKEN` or from `~/.netrc`, then creates a public repo. That is an environment variable and a file, then the GitHub API. The local `gh` Shim is not on that path. A token that `gh` has stored only in the Secret Service, and that is not also in the environment or in `hosts.yml`, is not what these two sources say the worm reads. This note does not claim the worm never execs `gh`. It claims the published primary descriptions do not depend on a `gh` process.

### 2.3 GitHub Actions secret dumps

[GHSA-mw4p-6x4p-x5m5](https://github.com/tj-actions/changed-files/security/advisories/GHSA-mw4p-6x4p-x5m5) on `tj-actions/changed-files` (CVE-2025-30066), published 22 March 2025. Tags `v1` through `v45.0.7` were moved on 14 and 15 March 2025 to commit `0e58ed8671d6b60d0890c21b07f8835ace038e67`. The action ran a script that read secrets from the Actions runner process memory and wrote them into the workflow log. CISA's 18 March 2025 alert (updated 26 March 2025) names the same CVE and also `reviewdog/action-setup@v1` (CVE-2025-30154), and says both were added to the Known Exploited Vulnerabilities catalog. The secrets include GitHub PATs, but the read happens inside a GitHub-hosted or self-hosted Actions runner. It is not a local `gh` call on an Omarchy machine, and this product does not patch CI.

### 2.4 A token already outside `gh`

Anything that already has the token can ignore the Shim:

- `curl` or any HTTP client to `api.github.com` with a token from the environment, from `~/.config/gh/hosts.yml`, or from a previous steal.
- Reading `hosts.yml` directly. `gh` writes the token there when secure storage fails. The gate does not wrap that file.
- `GH_TOKEN` / `GITHUB_TOKEN` in a parent process. The Shim overrides `GH_TOKEN` only in an allowed `gh` child.
- Another computer, or GitHub's own UI, using a token that already left the machine.

---

## 3. The report is about git, and it only matters if git is next

`git/git` published no security advisory in 2026 in the set returned by the repo advisory API (newest item is 8 July 2025). The 2025 items are below. This product does not intercept `git`. Each of these fires inside `git` itself.

One overlap exists today, and it is already a `gh` call. `gh auth setup-git` points Git's credential helper at `gh auth git-credential`. A later `git` fetch that asks that helper runs `gh`, and `auth git-credential get` is secret-reveal. The gate can refuse that helper call. It cannot refuse `git` when the helper is something else, and it cannot refuse the git bugs that never ask a helper.

| Advisory | Published | What git does | Why this gate does not stop it |
|---|---|---|---|
| [GHSA-hmg8-h7qf-7cxr](https://github.com/git/git/security/advisories/GHSA-hmg8-h7qf-7cxr) (CVE-2024-50349) | 14 Jan 2025 | Git prints an account name from the URL without stripping terminal control sequences, when it asks for credentials. | The print is inside `git`, not `gh`. |
| [GHSA-r5ph-xg7q-xfrp](https://github.com/git/git/security/advisories/GHSA-r5ph-xg7q-xfrp) (CVE-2024-52006) | 14 Jan 2025 | A carriage return in the credential-helper protocol can still confuse helpers that treat CR as a newline. Git can then send a stored credential to the wrong place. The 2020 fix (CVE-2020-5260) did not cover that. | The bug is in the git protocol and in the helper. If the helper is `gh`, section 1's secret-reveal rule can refuse `gh auth git-credential get`. Any other helper is untouched. |
| [GHSA-7jjc-gg6m-3329](https://github.com/git/git/security/advisories/GHSA-7jjc-gg6m-3329) (CVE-2024-52005) | 15 Jan 2025 | Git writes the sideband payload from the server to the terminal with no filtering. | The write is inside `git`. |
| [GHSA-vwqx-4fm8-6qc9](https://github.com/git/git/security/advisories/GHSA-vwqx-4fm8-6qc9) (CVE-2025-48384) | 8 Jul 2025 | A submodule path with a trailing CR is checked out on a different path. A symlink can point that path at the hooks directory, and a `post-checkout` hook in the submodule can then run. | `git submodule` / `git clone`. Not a `gh` call. `gh repo clone` is a separate, write-class `gh` command. This bug is the git checkout, not that command's classification. |
| [GHSA-m98c-vgpc-9655](https://github.com/git/git/security/advisories/GHSA-m98c-vgpc-9655) (CVE-2025-48385) | 8 Jul 2025 | A malicious bundle URI can inject protocol data so git writes the bundle to a path the server chooses. Worst case is code execution. Bundle URIs are off unless `bundle.heuristic` is set. | Inside `git clone` / fetch. |
| [GHSA-4v56-3xvj-xvfr](https://github.com/git/git/security/advisories/GHSA-4v56-3xvj-xvfr) (CVE-2025-48386) | 8 Jul 2025 | The Windows `wincred` helper overflows a static buffer. | Windows only. Not this Linux gate, even if git is next. |

The CVE ids for the January 2025 advisories say 2024. The vendor published those advisories on the January 2025 dates above. That is why they are in this note.

---

## What this note does not claim

- The gate does not fix a bug inside `gh`. It only decides whether that `gh` process starts.
- `Full` auto-allows the calls in section 1. A user who approves a prompt also lets the call through, once.
- A Path Shim does not see an absolute path to the real `gh` binary.
- CI, another workstation, and the GitHub website are out of scope. The Nx workflow injection and the Actions secret dumps ran there.
- No claim is made about `gh` advisories whose publish date is before 2025. The submodule token leak (GHSA-jwcm-9g39-pmcw, CVE-2024-53858, published 27 November 2024) is one of those, so it is not scored here. The commands it names (`gh repo clone`, `gh repo fork`, `gh pr checkout`) are write in the current table, which means a Read Launcher would prompt, but that is not a 2025 or 2026 report.

---

## Sources

Accessed 2026-09-27.

Gate behavior, this repo:

- `internal/ghclassify/ghclassify.go` - command class table, including `auth token` and `api` as secret-reveal, `auth status` as read unless `--show-token`, `run view` and `codespace ports` as read, and no rows for `attestation`, `release verify`, `codespace jupyter`, `codespace logs`, `skills`, or `agent-task`.
- `internal/policy/decide.go` - Deny never prompts. Read auto-allows only read. Trusted adds write. `Full` adds secret-reveal and unknown.
- `internal/shimexec/shimexec.go` - exec only after an allow. Secret-reveal does not offer a session grant. On allow, ambient `GH_TOKEN` is removed and the vault token is set. On vault release failure, `gh` keeps its own token lookup.
- `internal/identity/identity.go` - ancestry walk skips shells only. `npm` and `node` are not in that skip list.
- `docs/architecture.md` - Occupied Shim versus Path Shim, and the rule that the credential is handed only to the approved child.
- `README.md` - `cw policy enroll` defaults to Read.

s1ngularity:

- https://github.com/nrwl/nx/security/advisories/GHSA-cxm3-wv7p-598c - vendor advisory, published 27 August 2025. File scan, credential post, `s1ngularity-repository`, `/tmp/inventory.txt`, shell shutdown lines, the `pull_request_target` injection, commit `3905475cfd0e0ea670e20c6a9eaeb768169dc33d`, and the Nx Console install path.
- https://www.stepsecurity.io/blog/supply-chain-security-alert-popular-nx-build-system-package-compromised-with-data-stealing-malware - full `telemetry.js`, including `spawnSync('gh', ['auth', 'token'])`, the `gho_` / `ghp_` check, `process.env`, `~/.npmrc`, the AI CLI prompts, and `https.request` to `api.github.com`. Also the 28 August 2025 second wave. The script's prompt matches the prompt in the vendor advisory appendix.
- https://github.com/nrwl/nx/issues/32522 - the public report the vendor cites.

Shai-Hulud:

- https://www.cisa.gov/news-events/alerts/2025/09/23/widespread-supply-chain-compromise-impacting-npm-ecosystem - CISA alert, 23 September 2025. Credential scan, upload through the GitHub `/user/repos` API, npm propagation.
- https://github.blog/security/supply-chain-security/strengthening-supply-chain-security-preparing-for-the-next-malware-campaign/ - GitHub, 23 December 2025. Install-time token scavenging and propagation. No local `gh` process is described.
- https://www.stepsecurity.io/blog/sha1-hulud-the-second-coming-zapier-ens-domains-and-other-prominent-npm-packages-compromised - StepSecurity, 24 November 2025 wave. Token taken from `GITHUB_TOKEN` or `~/.netrc`.

GitHub Actions:

- https://github.com/tj-actions/changed-files/security/advisories/GHSA-mw4p-6x4p-x5m5 - CVE-2025-30066, published 22 March 2025.
- https://www.cisa.gov/news-events/alerts/2025/03/18/supply-chain-compromise-third-party-tj-actionschanged-files-cve-2025-30066-and-reviewdogaction - CISA alert, 18 March 2025, updated 26 March 2025. CVE-2025-30066 and CVE-2025-30154.

`gh` advisories, 2025 and 2026, from `cli/cli`:

- https://github.com/cli/cli/security/advisories/GHSA-fgw4-v983-mgp8 - CVE-2025-25204, 14 February 2025.
- https://github.com/cli/cli/issues/10418 - the report that advisory cites.
- https://github.com/cli/cli/security/advisories/GHSA-crc3-h8v6-qh57 - CVE-2026-45803, 13 May 2026.
- https://github.com/cli/cli/security/advisories/GHSA-8xvp-7hj6-mcj9 - CVE-2026-48501, 27 May 2026.
- https://github.com/cli/cli/security/advisories/GHSA-8cg3-r6g9-fpg2 - CVE-2026-59831, 2 July 2026.
- https://github.com/cli/cli/security/advisories/GHSA-4fjg-2h4q-fwg3 - CVE-2026-64653, 31 July 2026.
- https://github.com/cli/cli/security/advisories/GHSA-3m3g-3wcr-px46 - CVE-2026-64654, 31 July 2026.
- https://github.com/cli/cli/security/advisories/GHSA-cg6r-mpgc-h9mm - CVE-2026-64652, 31 July 2026.
- https://github.com/cli/cli/security/advisories/GHSA-mm27-mwq9-fr5g - CVE-2026-64655, 31 July 2026.
- https://github.com/cli/cli/security/advisories/GHSA-vfhh-p7hm-pxfh - CVE-2026-72924, 20 August 2026.

The list of `cli/cli` advisories was taken from `GET /repos/cli/cli/security-advisories`. The 2025 and 2026 published items are the ten above. Older `cli/cli` advisories are out of this note's date range.

Git advisories, from `GET /repos/git/git/security-advisories`. Newest published item is 8 July 2025. No 2026 item was in that list.

- https://github.com/git/git/security/advisories/GHSA-hmg8-h7qf-7cxr - CVE-2024-50349, published 14 January 2025.
- https://github.com/git/git/security/advisories/GHSA-r5ph-xg7q-xfrp - CVE-2024-52006, published 14 January 2025.
- https://github.com/git/git/security/advisories/GHSA-7jjc-gg6m-3329 - CVE-2024-52005, published 15 January 2025.
- https://github.com/git/git/security/advisories/GHSA-vwqx-4fm8-6qc9 - CVE-2025-48384, published 8 July 2025.
- https://github.com/git/git/security/advisories/GHSA-m98c-vgpc-9655 - CVE-2025-48385, published 8 July 2025.
- https://github.com/git/git/security/advisories/GHSA-4v56-3xvj-xvfr - CVE-2025-48386, published 8 July 2025.
