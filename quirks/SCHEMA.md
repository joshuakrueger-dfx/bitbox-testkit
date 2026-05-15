# quirks.json schema

The single source of truth for every documented BitBox02 firmware constraint.

**Canonical file:** `/go/bitbox/quirks/quirks.json` (must live inside the Go module so it can be embedded via `//go:embed`).

The Go loader (`/go/bitbox/quirks/loader.go`) embeds and parses this file at init time, attaching language-specific Scenario/Detect callbacks by quirk ID. The TypeScript loader (`/ts/src/quirks/loader.ts`) reads a synchronised copy at `/ts/src/quirks/quirks.json` — a parity test in both languages ensures the copy stays byte-identical.

## Top-level fields

| Field | Type | Notes |
|-------|------|-------|
| `schema_version` | string | semantic version of the schema |
| `description` | string | short summary, no semantic meaning |
| `quirks` | array | the entries — see below |

## Quirk entry fields

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `id` | string | yes | Stable identifier. Prefix matches category (E*=ETH, B*=BTC, C*=Cardano, M*=Mnemonic, P*=Protocol, A*=App). Must be unique. |
| `name` | string | yes | kebab-case slug, suitable for log lines. |
| `category` | string | yes | One of `eth`, `btc`, `cardano`, `mnemonic`, `protocol`, `app`. |
| `severity` | string | yes | One of `hint`, `warning`, `critical`. |
| `description` | string | yes | One-paragraph human explanation. |
| `source` | string | yes | Citation (proto file, CHANGELOG version, observed-in-production note). |
| `firmware.min` | string | yes | Inclusive lower bound (e.g. `"9.15.0"`). Empty means no minimum. |
| `firmware.max` | string | yes | Exclusive upper bound (e.g. `"9.15.0"`). Empty means no maximum. |
| `match_regex` | string | no | Regex matching test failure output that could indicate this quirk. Used by audit runners. |
| `detect` | array | no | Data-driven static-detection rules. Each rule is one of three kinds — see below. Empty/missing means the quirk has no static signature and can only be caught by runtime tests. |

## Detection rules (`detect`)

Each entry in the `detect` array has a `kind` field that decides which other fields are read.

### `kind: "regex"`

Simple line-level match. Flag every line of every in-scope file whose content matches `regex`.

| Field | Required | Notes |
|-------|----------|-------|
| `regex` | yes | RE2-compatible pattern. |
| `file_globs` | no | Array like `["*.ts", "*.tsx"]`. Default: all source files. |
| `reason` | yes | Human description of what was found. |
| `fix_hint` | no | One-line suggested remediation. |

### `kind: "regex_in_context"`

Two-pass match. File must first contain `context_regex` somewhere (anywhere — header import, comment, etc.), then per-line `regex` is applied. Used to suppress noise: an umlaut string only matters in a file that touches EIP-712 signing.

| Field | Required | Notes |
|-------|----------|-------|
| `regex` | yes | Per-line pattern. |
| `context_regex` | yes | File-level filter. |
| `file_globs` | no | |
| `reason` | yes | |
| `fix_hint` | no | |

### `kind: "ordered_pair"`

Order-of-occurrence check. The file must contain both `before_regex` and `after_regex`; a finding is emitted when `after_regex` is matched at an earlier byte offset than `before_regex`. Used for "X must happen before Y" patterns (e.g. dedup-check before set-clear).

| Field | Required | Notes |
|-------|----------|-------|
| `before_regex` | yes | Pattern that must appear first. |
| `after_regex` | yes | Pattern that must appear after. |
| `file_globs` | no | |
| `reason` | yes | |
| `fix_hint` | no | |

## Adding a new quirk

1. Pick the next free ID for the category.
2. Add entry to `quirks` array.
3. Add language-specific Scenario callback in `/go/bitbox/quirks/<category>.go` and `/ts/src/quirks/<category>.ts`.
4. (Optional) Add Detect callback if a static source pattern can flag the bug class.
5. Bump `schema_version` if you change the field shape — not when you just add entries.

## Severity guide

- **hint** — useful to know, no immediate hazard
- **warning** — wrong but recoverable, may surface as user-visible error
- **critical** — silent data loss, crash, or security regression
