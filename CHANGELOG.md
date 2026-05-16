# Changelog

All notable changes to bitbox-testkit. The project uses semantic versioning starting from v0.1.0.

## v0.5.0 — 2026-05-16

The "world-class" release: the baseline grows from 9 to 14 scenarios, covers Bitcoin alongside Ethereum, asserts a deterministic identity contract for the simulator seed, and now ships a multi-firmware matrix runner. The testkit self-audit is now green on its own source tree.

### Added

- **5 new simulator scenarios** in `go/bitbox/simulator/scenarios.go`:
  - `RootFingerprintDeterministic` — pins the simulator's BIP-32 root fingerprint to `0x4c00739d`. If this fails, every other pinned-output assertion downstream is suspect, so consumers see one canonical "seed drifted" red signal instead of N derived symptoms.
  - `EthSignLegacyPolygonMultiByteV` — actually drives `ETHSign(chainId=137)` to exercise the EIP-155 multi-byte v path (quirk CC-5). The pre-existing `EthAddressPolygonMultiByteV` only queried an address, which is identical regardless of chain id and therefore never tested the v-byte boundary.
  - `BtcXpubZpubMainnet` — BIP-84 native-segwit ZPUB shape (zpub-prefix + base58 length envelope).
  - `BtcAddressP2WPKHMainnet` — bech32 `bc1q` receive address at `m/84'/0'/0'/0/0`.
  - `BtcAddressP2TRTaproot` — bech32m `bc1p` Taproot address at `m/86'/0'/0'/0/0` (distinct firmware codepath from P2WPKH).
  - `BtcSignMessageMainnet` — Bitcoin signed-message envelope (64-byte R||S, recId 0..3, 65-byte Electrum sig with header byte 31..34).
- **`simulator.Connect` helper** factors the Noise XX bring-up + channel-hash-verify wait out of the CLI so the integration test, `cmd/bitbox-simulator-check`, and any future consumer share one canonical implementation. Tunable via `ConnectOptions{HandshakeTimeout, Logger}`.
- **`simulator.LaunchVersion(cacheDir, name)` + `ErrSimulatorNotFound`** lets a caller pin a specific embedded binary instead of always getting the newest one.
- **`bitbox-simulator-check --firmware <name|all>`** flag: matrix-runs the full baseline against every embedded firmware (v9.19.0 → v9.26.1) and emits a `MatrixReport` with a per-firmware pass/fail table. Single-firmware runs remain shape-compatible (`MatrixReport.Reports[0]` is the legacy `Report`).
- **CI: `go-simulator-matrix` job** in the testkit's own `test.yml` runs the baseline against each of the 8 embedded firmwares in parallel on every push, catching regressions that only surface on the long tail of older firmwares users still have in production.
- **CI: `TestSimulatorBaselineScenarios`** integration test executes every scenario against the real firmware on every push, replacing the previous "Launch only" smoke check. Surfaces scenario regressions at testkit-CI time instead of consumer time.

### Changed

- `cmd/bitbox-simulator-check`: report wire format is now `MatrixReport { Reports: [...] }` even for single-firmware runs. The CLI's exit-code semantics are unchanged (max of per-firmware exit codes).
- Umlaut KYC fixture payload now uses JSON `ü` / `ß` escapes rather than literal non-ASCII bytes. Equivalent at the wire layer (JSON parser resolves the escapes), but keeps the source file pure ASCII so the testkit's own self-audit stays green without `audit-skip-line` markers cluttering the file.
- TS `FakePairedBitBox`: proxy now ignores symbol-keyed lookups and returns `undefined` for `then` / `catch` / `finally`, so awaiting the proxy no longer infects the awaiter chain as if the proxy were thenable. Plus new `clearCalls()` for tests that want to reset the recorded call log mid-flight without releasing the fake.

### Fixed

- `audit-skip-line` marker (added in v0.4.4) is now actually wired through `detect.go`'s post-scan filter — previous versions documented the marker but never honoured it, so the 3 false-positive critical findings on dfx-wallet's `bitbox.ts:605` and `types.ts:62` doc comments leaked into every audit run.
- Testkit `quirks.test.ts` reads `quirks.json` directly instead of asserting a hardcoded count, so adding a quirk no longer requires a paired test-edit (the previous "expected 30, got 31" red was the symptom).

[v0.5.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.5.0

## v0.4.4 — 2026-05-16

`audit-skip-line` marker is now actually implemented in the audit-runner's scan loop. Before this it was documented for four releases (v0.4.0–v0.4.3) but the detector silently ignored the marker, so doc comments demonstrating an anti-pattern got flagged as real code. Particularly affected dfx-wallet's `bitbox.ts:605` and `types.ts:62`.

### Added

- `cmd/bitbox-audit/detect.go`: `isLineSuppressed` honours `audit-skip-line` on the offending source line OR the line directly above (matches the natural "comment on the line above" pattern). Three new tests in `audit_test.go` lock the behaviour.

[v0.4.4]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.4.4

## v0.4.3 — 2026-05-16

Action default bump only — composite `bitbox-simulator` action now defaults `testkit-ref` to v0.4.2, so consumers who pin the action ref without overriding the CLI version pick up the umlaut-reject scenario automatically.

[v0.4.3]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.4.3

## v0.4.2 — 2026-05-16

### Added

- `EthSignTypedDataNonAsciiRejected` scenario: feeds the 13-field RealUnitUser EIP-712 payload with German umlauts (ü, ß) and asserts the BitBox firmware REJECTS with ErrInvalidInput101. Pins the quirk-E1 firmware contract — a future firmware that silently started accepting non-ASCII would make consumer-side `toBitboxSafeAscii` transliteration load-bearing on one firmware version and dead code on the next. Failing-as-expected here is the GREEN state.

[v0.4.2]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.4.2

## v0.4.1 — 2026-05-16

Action default bump only — `testkit-ref` default → v0.4.0.

[v0.4.1]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.4.1

## v0.4.0 — 2026-05-16

### Added

- `EthSignTypedDataKycMultiPage` scenario: signs the exact 13-field RealUnitUser EIP-712 typed-data realunit-app's KYC onboarding uses. On a physical BitBox each string field renders as its own confirmation page ("1/13" → "13/13"); the simulator auto-confirms each page but the firmware still walks the full multi-page state machine. Guards the BLE-Dedup-Bug code path (1/13 → 2/13 transition, fixed 2026-05-14) and the antiklepto host-nonce-commitment exchange.

[v0.4.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.4.0

## v0.3.9 — 2026-05-16

### Fixed

- Composite `bitbox-simulator` action: sticky-comment step is now gated on `comment-on-pr && pull_request && head.repo.full_name == github.repository`, so fork PRs (which never get a write-scope token) no longer fail the whole job on the comment step. `continue-on-error: true` belt-and-braces.

[v0.3.9]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.9

## v0.3.8 — 2026-05-16

### Fixed

- `bitbox-simulator-check`: EIP-1559 scenario now uses a realistic payload (≈0.53 ETH at 6 gwei to a real-looking recipient) instead of zero-everything. The firmware refuses zero-recipient + zero-value + zero-gas combinations as obviously-malformed; the previous payload masked itself as a "firmware bug" when in fact every value being zero is the bug.

[v0.3.8]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.8

## v0.3.7 — 2026-05-16

### Fixed

- `bitbox-simulator-check`: switched simulator bring-up from `SetPassword(32)` to `RestoreFromMnemonic()` (upstream test pattern). `SetPassword` puts the device into a "showing newly-generated mnemonic" state, after which every ETH endpoint rejected calls with "can't call this endpoint: wrong state".

[v0.3.7]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.7

## v0.3.6 — 2026-05-16

Action default bump → v0.3.5 (handshake fix).

[v0.3.6]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.6

## v0.3.5 — 2026-05-16

### Fixed

- `bitbox-simulator-check`: after `firmware.Device.Init()` we now poll `ChannelHash()` until the simulator reports `verified=true`, then call `ChannelHashVerify(true)`. Without this step every post-pair API call fails with "handshake must come first" — `Init` alone is not sufficient for the simulator firmware to unlock its endpoint surface.

[v0.3.5]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.5

## v0.3.4 — 2026-05-16

Action default bump.

[v0.3.4]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.4

## v0.3.3 — 2026-05-16

### Fixed

- `go/core/simulator/simulator.go`: hash-mismatch error now surfaces the expected vs actual SHA-256 in the error message. Previously the error said only "hash mismatch", giving no signal whether upstream had reproducibly rebuilt the artefact or whether a real supply-chain alarm was firing.

[v0.3.3]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.3

## v0.3.2 — 2026-05-16

Re-release of v0.3.1 from a fresh commit. Go's module proxy caches by version+commit-hash, so force-retagging the original v0.3.1 commit after a botched embedded.go update did not propagate to consumers via `go install`. v0.3.2 is the consumer-visible "v0.3.1 with the SHA pins actually correct".

[v0.3.2]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.2

## v0.3.1 — 2026-05-16

Patch: refresh SHA-256 pins for the three most-recent simulator binaries (v9.24.0, v9.25.0, v9.26.1) after Shift Crypto reproducibly rebuilt the upstream artefacts. Behaviour is unchanged — the rebuild only altered build-metadata (timestamps, paths). The five older versions (v9.19.0–v9.23.0) still match their original pins.

Caught by the first CI run of `bitbox-simulator-check` against `bitbox-api@0.12.0` on PR #153 — exactly the supply-chain-alarm mode the script was designed for. No user action required beyond bumping the testkit ref in consumer workflows from v0.3.0 → v0.3.1.

[v0.3.1]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.1

## v0.3.0 — 2026-05-16

End-to-end against real firmware: a new CLI launches the official BitBox02 simulator binary and runs the testkit's curated baseline scenarios — pair, deviceInfo, get-address (mainnet + Polygon multi-byte v), sign-message (small + 1024-byte boundary), sign-EIP1559 — against the actual firmware logic. Replaces mock-only test coverage with real-firmware validation for every consumer.

### Added

- **`bitbox-simulator-check` CLI** (`go/cmd/bitbox-simulator-check`): launches the simulator from the embedded SHA-pinned list (currently v9.19.0 through v9.26.1), exercises BaselineScenarios, emits JSON or Markdown report, exits non-zero on scenario failure. Linux/amd64 only — on macOS / Windows / arm Linux the CLI exits cleanly with a "skipped" status so cross-platform CI matrices stay simple.
- **`go/bitbox/simulator/scenarios.go`**: typed Scenario library. BaselineScenarios() returns the curated set every DFX consumer cares about — extendable by future quirks.
- **Composite action `.github/actions/bitbox-simulator/action.yml`**: one-line `uses:` step for any DFX repo. Inputs: `testkit-ref`, `comment-on-pr`, `fail-on-findings`, `fail-on-skip`.
- **Workflow templates** `bitbox-simulator.yml` (auto-trigger) and `bitbox-simulator-slash.yml` (`/bitbox-simulator` comment trigger with author_association auth gate).

### Changed

- ONBOARDING §4 adds a third recommended workflow alongside `bitbox-audit` and the slash command.

[v0.3.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.0

## v0.2.0 — 2026-05-16

Reusable distribution: one `uses:` step gives any DFX repo a fully wired BitBox audit. Quirk count climbs from 30 → 31; static detection from 7 → 11.

### Added

- **Composite GitHub Action** `joshuakrueger-dfx/bitbox-testkit/.github/actions/bitbox-audit@v0.2.0`: encapsulates Go + Node setup, dependency install, Jest `--json`, audit CLI install + run, sticky PR comment, artifact upload. Inputs cover firmware narrowing, `jest-extra-args`, `fail-on-findings`, testkit ref pinning, WASM hash check toggle.
- **Slash-command template** `bitbox-audit-slash.yml`: a maintainer comments `/bitbox-audit [firmware=X] [ref=Y] [fail]` on any PR to trigger an on-demand audit. Authorization gates on `author_association ∈ {OWNER, MEMBER, COLLABORATOR}`.
- **Workflow templates** registered as `.github/workflow-templates/` so GitHub's "New workflow" UI lists them for any org-owned repo.
- **Quirk A4** — address-display-on-device-required (consumer must call `BTCAddress(displayOnDevice=true)` for receive flows, else man-in-the-middle on `getAddress` JSON-RPC).
- **Static detection** for quirks B2, C2, A3, A4. Coverage now 11 of 31 (E1, E7, B1, B2, C2, M1, P2, A1, A2, A3, A4).
- **CI: composite action self-test job** runs the action against the testkit itself to catch regressions in the action's own shell logic. YAML-lint job parses every workflow/action under `.github/`.

### Changed

- **ONBOARDING.md** §4 rewritten: composite-action `uses:` line is now the recommended path; long-form workflow demoted to fallback.
- **A1 detection** narrowed from "any `//export`" to "any `//export` without a paired `defer recoverPanic(` within 6 lines" via the new `missing_pair_within` rule kind — fixed false-positive on every cgo binding in `bitbox_flutter`.
- **A2 detection** narrowed via `context_regex` so `setTimeout` no longer matches; only BitBox / U2FHID / hardware-wallet / signing call sites are considered.

### Fixed

- `regex_in_context` no longer required RE2 lookahead (`(?!\s*\*)`), which Go's regexp engine doesn't support.

## v0.1.0 — 2026-05-15

First release. Establishes the architecture and ships an initial knowledge base of 30 documented BitBox02 firmware constraints.

### Capabilities

- **Quirks knowledge base** (`/go/bitbox/quirks/quirks.json`): 30 quirks across ETH (10), BTC (7), Cardano (4), Mnemonic (3), Protocol (3), App (3). Each carries severity, firmware version range, source citation, and — where statically detectable — at least one detection pattern.
- **Data-driven static detection** with four rule kinds: `regex`, `regex_in_context`, `ordered_pair`, `missing_pair_within`. Seven of the 30 quirks ship with detection patterns: E1, E7, B1, M1, P2, A1, A2.
- **`bitbox-audit` CLI** scans any repository (Go, TypeScript/JS, Dart) and emits JSON or Markdown reports. Coverage-aware: distinguishes statically-detected quirks from runtime-only ones, surfaces explicit "not covered" lists so absence of findings doesn't masquerade as completeness.
- **`bitbox-audit-explain` CLI** turns audit JSON into a plain-language narrative. Calls the Anthropic Messages API when `ANTHROPIC_API_KEY` is set; prints the structured prompt otherwise.
- **Test-results integration**: `--test-results <path>` accepts Jest `--json` or `go test -json` output. Quirks named in passing/failing test descriptions get folded into the Coverage table.
- **Per-file inline suppression** via `audit-skip-line` comments; whole-file via `audit-skip-file`.

### Go testkit (`/go`)

- `bitbox/fake` — scriptable in-memory implementation of `firmware.Communication` with handler chains, recorded calls, defensive snapshots.
- `bitbox/scenarios` — eight pre-built scenarios: Umlaut-EIP712, Disconnect, Panic, SlowResponse, ChannelHashEarly, ErrInvalidInput, UnknownNetwork, PairingRace.
- `bitbox/simulator` — official BitBox02 simulator binary lifecycle (Linux/amd64 only). Embedded list covers firmware v9.19.0 through v9.26.1 with SHA256 verification.
- `core/transport/ble` — `io.ReadWriteCloser` BLE peripheral fake. Race-tested with 5000-byte interleaved Inject/Read and 50-trial Close races.
- `core/guards` — test-time wrappers that lift static findings to `testing.TB.Errorf`. Data-driven from the same quirks.json the audit-runner uses, so the two surfaces cannot drift.
- `core/testutil` — deadlock detector, timeout-bounded execution, atomic counter, polling-based assertions.

### TypeScript testkit (`/ts`)

- `fake/` — `FakePairedBitBox` with Proxy-based dispatch. Symbol-safe (does not pretend to be thenable), records calls, supports `clearCalls` mid-flight.
- `scenarios/` — eight scenario factories matching the Go set.
- `guards/` — `expandGlobs` + per-quirk source-level checks for TypeScript / JavaScript codebases.
- `quirks/` — `Registry`, `subset`, `firmwareApplies`. Loads the canonical JSON shared with the Go side (kept byte-identical by `scripts/sync-quirks.sh`).

### CI integration

- `.github/workflows/test.yml`: three jobs — quirks-sync verification, Go vet + race tests, TypeScript unit tests.
- Worked example in `DFXswiss/dfx-wallet#153`: PR-triggered audit job that runs Jest in `--json` mode, feeds results to bitbox-audit, posts a sticky PR comment with static + dynamic coverage.

### Validated against

- `DFXswiss/dfx-wallet` (React Native + TypeScript): 223 files scanned, 0 false positives, 13 of 30 quirks covered (7 static + 10 runtime, 4 overlapping). The 16 uncovered are not reachable in dfx-wallet's current BitBox surface.
- `DFXswiss/bitbox_flutter` (Flutter plugin, Go-heavy): 20 files scanned, 0 false positives. The `missing_pair_within` A1 detector correctly recognises every `//export` declaration's paired `defer recoverPanic()`.
- `DFXswiss/realunit-app` (pure Dart): 408 files scanned, 0 false positives. Dart-shaped patterns (`Future.delayed(Duration(seconds: 10))`) added for A2.
- Testkit self-scan: 0 false positives after introducing `audit-skip-file` markers on the pattern-documenting guard files.

### Known limitations (intentional)

- No npm publish yet; TypeScript consumers either vendor `/ts/` or install via the git URL.
- 16 of the 30 quirks have no static signature; coverage relies on consumer-written runtime tests with the testkit's scenarios.
- macOS / Windows BitBox02 simulator builds do not exist upstream; the Linux/amd64 simulator integration is the only end-to-end runtime path.
- Vendor-firmware SHA256s for v9.24.0–v9.26.1 were transcribed from the BitBoxSwiss releases page; a CI download surfaces an explicit hash-mismatch error if upstream rebuilds an artifact, rather than silently substituting tampered content.

[v0.1.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.1.0
[v0.2.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.2.0
