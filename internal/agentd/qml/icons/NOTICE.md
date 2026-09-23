# Icon provenance

`claudecode.svg`, `gemini.svg`, `github.svg`, `git.svg`, and `docker.svg` are
vendored from [Simple Icons](https://simpleicons.org) (package `simple-icons`,
version 16.32.0), whose icon path data is released under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/). Only the `fill`
color was changed from the upstream files; the path geometry is unmodified.

| File | Upstream slug | Fill |
|---|---|---|
| claudecode.svg | `claudecode` | `#D97757` |
| gemini.svg | `googlegemini` | `#8E75B2` |
| github.svg | `github` | `#ffffff` |
| git.svg | `git` | `#F03C2E` |
| docker.svg | `docker` | `#2496ED` |

The names and logos above are trademarks of their respective owners (Anthropic
PBC, Google LLC, GitHub Inc., the Git project, Docker Inc.). Simple Icons' CC0
license covers the redistributed vector files only, not a grant to use the
trademark itself — usage here is purely as a small in-app glyph identifying
which agent/tool a request is going through, the same way Simple Icons is used
by countless other open-source dev tools.

## openai.svg and azure.svg — vendored outside their stated terms, by explicit request

Both were flagged as sitting outside their brand owner's actual permitted use,
and the user chose to vendor them anyway rather than use the neutral
fallback. If this repo's owner changes their mind, replace either file with
`unknown.svg`'s contents — don't just delete the mapping and leave a broken
`Image.source`.

- **openai.svg** (fill `#ffffff`): the `openai` slug's path data, fetched
  from Simple Icons' CDN even though the currently released package (since
  somewhere between v15.22.0 and v16.31.0) no longer lists it — a removal of
  that kind is "usually a response to a request from the brand owner rather
  than a cleanup."
- **azure.svg** (fill `#0078D4`, Microsoft's brand blue): the `microsoftazure`
  slug's path data, same stale-CDN situation as openai.svg (Simple Icons
  currently carries no Microsoft-family marks at all — Azure, VS Code, Edge,
  etc. are all absent). Separately, Microsoft's own Azure Architecture Center
  icon terms explicitly permit use only in "architectural diagrams, training
  materials, or documentation" — a live running UI widget (this popup, not a
  diagram or doc) isn't clearly one of those three.

## Deliberately not vendored — do not add this back without re-checking

- **Grok / xAI**: no CC0/license-clear source exists — never had a Simple
  Icons entry, and the only hits are third-party logo-aggregator mirrors of
  xAI's trademarked mark (logo.dev, brandfetch, seeklogo, etc.), not a
  redistributable asset. Falls back to `unknown.svg`. If xAI ever publishes
  an official brand-kit SVG under clear usage terms, vendor from there
  directly — don't pull from a scraper site.

If Grok ever needs a real icon, don't hand-draw an approximation of the
trademarked mark — that reintroduces the exact ambiguity vendoring from
Simple Icons was meant to avoid in the first place.

`unknown.svg` is an original, neutral placeholder used for any launcher or
tool CmdWarden-Omarchy doesn't have an icon for.
