# Contributing to bitbox-testkit

The kit's value scales with the size of the quirks knowledge base. If you discover a new BitBox02 firmware constraint — a quirk that bit your code or a customer's — please file it back into the JSON so every consumer picks up coverage on their next install.

## Adding a new quirk

1. **Pick an ID.** Choose the next free slot in the category (e.g. `E11` for an 11th ETH quirk). Categories: `eth`, `btc`, `cardano`, `mnemonic`, `protocol`, `app`.

2. **Edit `/go/bitbox/quirks/quirks.json`.** Add an entry with:

   ```jsonc
   {
     "id": "E11",
     "name": "kebab-case-slug",
     "category": "eth",
     "severity": "critical",                       // hint | warning | critical
     "description": "One-paragraph human explanation.",
     "source": "messages/eth.proto: …  or  CHANGELOG v9.X.Y: …  or  observed in production",
     "firmware": { "min": "9.0.0", "max": "" },     // empty = unbounded
     "match_regex": "(?i)…",                         // optional: test-output classifier
     "detect": [                                     // optional: static-detection rules
       { "kind": "regex", "regex": "…", "reason": "…", "fix_hint": "…" }
     ]
   }
   ```

   See [`quirks/SCHEMA.md`](quirks/SCHEMA.md) for the full schema.

3. **Sync to the TypeScript side:**

   ```bash
   ./scripts/sync-quirks.sh
   ```

   The TS loader reads the synced copy at `/ts/src/quirks/quirks.json`. CI runs `--check` and fails on drift.

4. **(Optional) Add a Scenario factory** if the firmware response shape differs from the generic `ErrInvalidInput101` rejection.

   - `/go/bitbox/scenarios/scenarios.go` — Go side.
   - `/ts/src/scenarios/index.ts` — TS side.
   - Wire into `/go/bitbox/quirks/callbacks.go` and `/ts/src/quirks/callbacks.ts` `attachCallbacks`/`switch q.id` arms so the quirk's `Scenario` field gets your factory instead of the default.

5. **Add tests for the new detection rule** in `/go/cmd/bitbox-audit/audit_test.go`. Include both a positive case (pattern fires correctly) and a negative case (similar code that should NOT fire). Without the negative test, false-positive regressions creep in.

6. **Re-validate against real repos.** Run `bitbox-audit --repo <somewhere-with-bitbox-code>` against at least one consumer that exercises the new quirk's surface. False positives here cost more than missing detection — a noisy audit gets muted by consumers.

7. **Update `CHANGELOG.md`** under the next-release section with a one-line entry.

## Detection rule kinds (cheat sheet)

| Kind                 | When to use                                                                 |
| -------------------- | --------------------------------------------------------------------------- |
| `regex`              | Simple line-level match; no per-file gating needed.                         |
| `regex_in_context`   | Match `regex` only in files whose content satisfies `context_regex`. Suppresses noise where the same surface text means different things in different contexts. |
| `ordered_pair`       | "X must appear before Y." File must contain both `before_regex` and `after_regex`; a finding is emitted only when `after_regex` precedes `before_regex` in byte offset. |
| `missing_pair_within`| "Every X must be followed by Y within N lines." Use for export-with-guard, lock-with-defer-unlock, and other proximity-paired patterns. |

Regex compatibility: Go uses RE2 (no lookahead / lookbehind / backreferences). JS regex is RE2-compatible for the patterns we need. Write regexes that work in both engines; the audit-runner uses Go RE2.

## Adding a Scenario factory

A Scenario returns a configured fake suitable for use in a single test. Two questions to ask:

1. **Does the firmware respond differently from a generic `ErrInvalidInput101`?** If no, reuse the default. Adding identical wrappers just adds noise.

2. **Is there a multi-step flow?** Use a closure that captures state (counter, confirm flag) and returns different responses on subsequent calls. See `scenarioChannelHashEarly` for the canonical example.

## Validating your changes locally

```bash
# Go: full test sweep with race detector
(cd go && go test -race -timeout 60s ./...)

# TS: typecheck + jest
(cd ts && npx tsc --noEmit && npm test)

# JSON sync gate (CI runs this as well)
./scripts/sync-quirks.sh --check

# Audit against your own working tree
(cd go && go run ./cmd/bitbox-audit --repo .. --format markdown)
```

## Releases

Tags follow `v0.MAJOR.MINOR` semver. The TypeScript package and the Go module both pick up the same tag — there's no separate cadence. CHANGELOG.md must be updated as part of the release commit.

The Go module lives at `/go/`, so Go's submodule-tagging convention requires **two** tags pointing at the same commit: `vX.Y.Z` for the repo / composite-action ref, and `go/vX.Y.Z` for `go install` to resolve the package. Without the `go/` prefixed tag, consumers hit:

> `module github.com/joshuakrueger-dfx/bitbox-testkit@vX.Y.Z found, but does not contain package …/go/cmd/bitbox-audit`

```bash
# Bump version in /ts/package.json
# Update CHANGELOG.md
git commit -am "Release vX.Y.Z"

# Two tags, one commit. The 'go/' prefix is required by Go's
# submodule resolver — see https://go.dev/ref/mod#vcs-version.
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git tag -a go/vX.Y.Z -m "go/vX.Y.Z: submodule tag matching vX.Y.Z" vX.Y.Z^{}

git push origin main --tags
```
