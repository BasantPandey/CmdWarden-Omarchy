// Package ghclassify maps a gh CLI invocation to its Command Class
// (read/write/secret-reveal/unknown), mirroring the classification table in
// CmdWarden's own Windows gh harden design
// (https://github.com/BasantPandey/CmdWarden/blob/main/docs/spec/cmdwarden.md#141-gh),
// adapted for Linux — same gh binary, same subcommands.
//
// This is a pure function over the subcommand argv: no I/O, no agent, no
// vault, no shim. It is called both by the real-time Shim and by any
// dry-run/test tooling (see internal/policy).
package ghclassify

import (
	"strings"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// table maps a gh subcommand path (space-joined, 1-3 tokens, e.g. "auth
// token" or "search prs") to its Command Class. Longest matching path wins;
// an unmatched path classifies as ClassUnknown rather than defaulting to
// ClassRead — see Classify.
var table = map[string]contracts.CommandClass{
	// auth: token-export and most auth-state-changing commands.
	"auth token":                contracts.ClassSecretReveal,
	"auth status":               contracts.ClassRead, // upgraded to secret-reveal by --show-token, see Classify
	"auth login":                contracts.ClassWrite,
	"auth refresh":              contracts.ClassWrite,
	"auth logout":               contracts.ClassWrite,
	"auth switch":               contracts.ClassWrite,
	"auth setup-git":            contracts.ClassWrite,
	"auth git-credential get":   contracts.ClassSecretReveal,
	"auth git-credential store": contracts.ClassWrite,
	"auth git-credential erase": contracts.ClassWrite,

	// repo
	"repo view":         contracts.ClassRead,
	"repo list":         contracts.ClassRead,
	"repo clone":        contracts.ClassWrite,
	"repo create":       contracts.ClassWrite,
	"repo delete":       contracts.ClassWrite,
	"repo fork":         contracts.ClassWrite,
	"repo edit":         contracts.ClassWrite,
	"repo sync":         contracts.ClassWrite,
	"repo archive":      contracts.ClassWrite,
	"repo unarchive":    contracts.ClassWrite,
	"repo rename":       contracts.ClassWrite,
	"repo set-default":  contracts.ClassWrite,
	"repo deploy-key":   contracts.ClassRead, // list-only subcommand; add/delete overridden below
	"repo autolink":     contracts.ClassRead,
	"repo gitignore":    contracts.ClassRead,
	"repo license":      contracts.ClassRead,
	"repo credits":      contracts.ClassRead,
	"repo add-remote":   contracts.ClassWrite,
	"repo pin":          contracts.ClassWrite,
	"repo unpin":        contracts.ClassWrite,
	"repo edit-default": contracts.ClassWrite,

	// pr
	"pr list":          contracts.ClassRead,
	"pr view":          contracts.ClassRead,
	"pr status":        contracts.ClassRead,
	"pr diff":          contracts.ClassRead,
	"pr checks":        contracts.ClassRead,
	"pr create":        contracts.ClassWrite,
	"pr merge":         contracts.ClassWrite,
	"pr close":         contracts.ClassWrite,
	"pr reopen":        contracts.ClassWrite,
	"pr edit":          contracts.ClassWrite,
	"pr review":        contracts.ClassWrite,
	"pr comment":       contracts.ClassWrite,
	"pr checkout":      contracts.ClassWrite,
	"pr ready":         contracts.ClassWrite,
	"pr lock":          contracts.ClassWrite,
	"pr unlock":        contracts.ClassWrite,
	"pr update-branch": contracts.ClassWrite,

	// issue
	"issue list":     contracts.ClassRead,
	"issue view":     contracts.ClassRead,
	"issue status":   contracts.ClassRead,
	"issue create":   contracts.ClassWrite,
	"issue close":    contracts.ClassWrite,
	"issue reopen":   contracts.ClassWrite,
	"issue edit":     contracts.ClassWrite,
	"issue comment":  contracts.ClassWrite,
	"issue lock":     contracts.ClassWrite,
	"issue unlock":   contracts.ClassWrite,
	"issue transfer": contracts.ClassWrite,
	"issue pin":      contracts.ClassWrite,
	"issue unpin":    contracts.ClassWrite,
	"issue delete":   contracts.ClassWrite,
	"issue develop":  contracts.ClassWrite,

	// gist
	"gist list":   contracts.ClassRead,
	"gist view":   contracts.ClassRead,
	"gist create": contracts.ClassWrite,
	"gist edit":   contracts.ClassWrite,
	"gist delete": contracts.ClassWrite,
	"gist clone":  contracts.ClassWrite,
	"gist rename": contracts.ClassWrite,

	// release
	"release list":         contracts.ClassRead,
	"release view":         contracts.ClassRead,
	"release download":     contracts.ClassRead,
	"release create":       contracts.ClassWrite,
	"release edit":         contracts.ClassWrite,
	"release delete":       contracts.ClassWrite,
	"release upload":       contracts.ClassWrite,
	"release delete-asset": contracts.ClassWrite,

	// secret / variable (org/repo/env secrets — values are write-only via gh anyway)
	"secret list":   contracts.ClassRead,
	"secret set":    contracts.ClassWrite,
	"secret delete": contracts.ClassWrite,

	"variable list":   contracts.ClassRead,
	"variable get":    contracts.ClassRead,
	"variable set":    contracts.ClassWrite,
	"variable delete": contracts.ClassWrite,

	// workflow / run (Actions)
	"workflow list":    contracts.ClassRead,
	"workflow view":    contracts.ClassRead,
	"workflow run":     contracts.ClassWrite,
	"workflow enable":  contracts.ClassWrite,
	"workflow disable": contracts.ClassWrite,

	"run list":     contracts.ClassRead,
	"run view":     contracts.ClassRead,
	"run watch":    contracts.ClassRead,
	"run download": contracts.ClassRead,
	"run cancel":   contracts.ClassWrite,
	"run rerun":    contracts.ClassWrite,
	"run delete":   contracts.ClassWrite,

	"cache list":   contracts.ClassRead,
	"cache delete": contracts.ClassWrite,

	// keys
	"ssh-key list":   contracts.ClassRead,
	"ssh-key add":    contracts.ClassWrite,
	"ssh-key delete": contracts.ClassWrite,
	"gpg-key list":   contracts.ClassRead,
	"gpg-key add":    contracts.ClassWrite,
	"gpg-key delete": contracts.ClassWrite,

	// org / project / label
	"org list": contracts.ClassRead,

	"project list":   contracts.ClassRead,
	"project view":   contracts.ClassRead,
	"project create": contracts.ClassWrite,
	"project edit":   contracts.ClassWrite,
	"project delete": contracts.ClassWrite,
	"project close":  contracts.ClassWrite,
	"project link":   contracts.ClassWrite,
	"project unlink": contracts.ClassWrite,

	"label list":   contracts.ClassRead,
	"label create": contracts.ClassWrite,
	"label edit":   contracts.ClassWrite,
	"label delete": contracts.ClassWrite,
	"label clone":  contracts.ClassWrite,

	// extension / alias / config (local, non-GitHub-state, but still
	// mutate the local gh install / shell — treated as write when they
	// mutate, read when they only inspect)
	"extension list":    contracts.ClassRead,
	"extension install": contracts.ClassWrite,
	"extension remove":  contracts.ClassWrite,
	"extension upgrade": contracts.ClassWrite,
	"extension create":  contracts.ClassWrite,

	"config get":         contracts.ClassRead,
	"config list":        contracts.ClassRead,
	"config set":         contracts.ClassWrite,
	"config clear-cache": contracts.ClassWrite,

	"alias list":   contracts.ClassRead,
	"alias set":    contracts.ClassWrite,
	"alias delete": contracts.ClassWrite,
	"alias import": contracts.ClassWrite,

	// codespace
	"codespace list":    contracts.ClassRead,
	"codespace view":    contracts.ClassRead,
	"codespace ports":   contracts.ClassRead,
	"codespace logs":    contracts.ClassRead,
	"codespace create":  contracts.ClassWrite,
	"codespace delete":  contracts.ClassWrite,
	"codespace stop":    contracts.ClassWrite,
	"codespace ssh":     contracts.ClassWrite,
	"codespace code":    contracts.ClassWrite,
	"codespace edit":    contracts.ClassWrite,
	"codespace rebuild": contracts.ClassWrite,

	// ruleset (read-only surface in gh today)
	"ruleset list":  contracts.ClassRead,
	"ruleset view":  contracts.ClassRead,
	"ruleset check": contracts.ClassRead,

	// search (always read)
	"search prs":     contracts.ClassRead,
	"search issues":  contracts.ClassRead,
	"search repos":   contracts.ClassRead,
	"search code":    contracts.ClassRead,
	"search commits": contracts.ClassRead,

	// misc top-level
	"browse": contracts.ClassRead,
	"status": contracts.ClassRead,

	// api: can reach arbitrary GitHub REST/GraphQL endpoints, including
	// ones that reveal secrets (e.g. Actions secrets metadata, deploy
	// keys) — classify conservatively as secret-reveal rather than
	// trying to parse the endpoint path.
	"api": contracts.ClassSecretReveal,
}

// Classify returns the Command Class for a gh invocation given the argument
// vector *following* the "gh" binary itself, e.g. []string{"pr", "create",
// "--title", "x"} or []string{"auth", "token"}. It never returns an error:
// anything it doesn't recognize is ClassUnknown.
func Classify(args []string) contracts.CommandClass {
	tokens := nonFlagTokens(args)

	for length := 3; length >= 1; length-- {
		if len(tokens) < length {
			continue
		}
		path := strings.Join(tokens[:length], " ")
		class, ok := table[path]
		if !ok {
			continue
		}
		return applyFlagOverrides(path, args, class)
	}

	return contracts.ClassUnknown
}

// nonFlagTokens returns the leading positional (non "-"-prefixed) arguments,
// which for gh are always the subcommand path — flags never appear before
// the full subcommand path is spelled out (e.g. "gh pr --repo x create" is
// not valid gh syntax).
func nonFlagTokens(args []string) []string {
	var tokens []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		tokens = append(tokens, a)
	}
	return tokens
}

// applyFlagOverrides upgrades a base classification when a specific flag
// changes what the command actually does — currently just `gh auth status
// --show-token`, which turns an otherwise-read command into secret-reveal.
func applyFlagOverrides(path string, args []string, base contracts.CommandClass) contracts.CommandClass {
	if path == "auth status" && hasFlag(args, "--show-token") {
		return contracts.ClassSecretReveal
	}
	return base
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
