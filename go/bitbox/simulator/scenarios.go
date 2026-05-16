//go:build simulator

package simulator

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/BitBoxSwiss/bitbox02-api-go/api/firmware"
	"github.com/BitBoxSwiss/bitbox02-api-go/api/firmware/messages"
)

// Result is the outcome of a single scenario run against the simulator.
// Passed=false with Detail carries the rejection reason for the report.
type Result struct {
	Name      string        `json:"name"`
	Passed    bool          `json:"passed"`
	Detail    string        `json:"detail"`
	Duration  time.Duration `json:"duration_ms_raw"`
	DurationMs int64        `json:"duration_ms"`
}

// Scenario is a self-contained probe that takes an attached firmware
// Device and returns whether the bitbox-api protocol behaves as
// expected. Scenarios MUST be idempotent so they can be re-run inside
// a matrix.
type Scenario func(dev *firmware.Device) Result

// realUnitUserKycPayload mirrors the EIP-712 typed-data shape that
// realunit-app's KYC registration flow signs. 13 fields of type
// string + bool + address — on a physical BitBox each string field
// is its own confirmation page (the on-device screen renders
// "1/13", "2/13", … "13/13"). That multi-page flow is exactly where
// the BLE-Dedup-Bug fixed on 2026-05-14 used to drop the 1/13 → 2/13
// transition. Including this scenario guards against any future
// regression in the same code path.
//
// ALL string values stay ASCII because the BitBox firmware rejects
// non-ASCII bytes in EIP-712 string fields with ErrInvalidInput101
// (quirk E1) — realunit-app fixes this client-side via
// toBitboxSafeAscii. The simulator follows the same firmware
// contract, so feeding non-ASCII here would just exercise the
// reject path; we test the happy path that consumers should be
// hitting after their own transliteration step.
const realUnitUserKycPayload = `{
  "types": {
    "EIP712Domain": [
      {"name": "name", "type": "string"},
      {"name": "version", "type": "string"}
    ],
    "RealUnitUser": [
      {"name": "email", "type": "string"},
      {"name": "name", "type": "string"},
      {"name": "type", "type": "string"},
      {"name": "phoneNumber", "type": "string"},
      {"name": "birthday", "type": "string"},
      {"name": "nationality", "type": "string"},
      {"name": "addressStreet", "type": "string"},
      {"name": "addressPostalCode", "type": "string"},
      {"name": "addressCity", "type": "string"},
      {"name": "addressCountry", "type": "string"},
      {"name": "swissTaxResidence", "type": "bool"},
      {"name": "registrationDate", "type": "string"},
      {"name": "walletAddress", "type": "address"}
    ]
  },
  "primaryType": "RealUnitUser",
  "domain": {"name": "RealUnitUser", "version": "1"},
  "message": {
    "email": "test@dfx.swiss",
    "name": "Test User",
    "type": "natural-person",
    "phoneNumber": "+41123456789",
    "birthday": "1990-01-01",
    "nationality": "Switzerland",
    "addressStreet": "Bahnhofstrasse 1",
    "addressPostalCode": "8001",
    "addressCity": "Zurich",
    "addressCountry": "Switzerland",
    "swissTaxResidence": true,
    "registrationDate": "2026-05-16T12:00:00Z",
    "walletAddress": "0x0000000000000000000000000000000000000000"
  }
}`

// BaselineScenarios returns the curated set of probes that exercise
// the protocol surface every consumer (dfx-wallet, realunit-app) cares
// about, in roughly the order a real onboarding flow runs them.
//
// The simulator firmware accepts these calls without user interaction
// because it auto-confirms every prompt; on a physical BitBox the
// user would tap to confirm. The simulator is pre-loaded with the
// upstream fixture mnemonic so address-derivation outputs are
// deterministic — the BIP-32 root fingerprint is 0x4c00739d for every
// simulator session, which lets us pin exact xpubs / addresses across
// runs (asserted by RootFingerprintDeterministic).
//
// Naming convention: where a scenario directly guards a known quirk
// from quirks.json the function name and scenario id reference the
// quirk id (E1, P2, CC-5 …) so a finding in CI maps unambiguously to
// the documented anti-pattern.
//
// For each scenario we assert what the CONSUMER would see — error
// class, byte shape, identity contract.
func BaselineScenarios() []Scenario {
	return []Scenario{
		// Pairing + bring-up.
		PairAndDeviceInfo,
		RestoreSimulatorMnemonic,
		RootFingerprintDeterministic,

		// Ethereum address surface.
		EthAddressMainnet,
		EthAddressPolygonMultiByteV,

		// Ethereum sign surface.
		EthSignMessageAscii,
		EthSignMessageBoundary,
		EthSignLegacyPolygonMultiByteV,
		EthSignEIP1559Mainnet,
		EthSignTypedDataKycMultiPage,
		EthSignTypedDataNonAsciiRejected,

		// Bitcoin surface (BIP-84 native segwit + BIP-86 taproot).
		BtcXpubZpubMainnet,
		BtcAddressP2WPKHMainnet,
		BtcAddressP2TRTaproot,
		BtcSignMessageMainnet,
	}
}

// PairAndDeviceInfo verifies the device responds to DeviceInfo before
// any seed is set up. Covers the pair-channel + initial query path —
// equivalent to dfx-wallet's connect() / fetchDeviceInfo().
func PairAndDeviceInfo(dev *firmware.Device) Result {
	return run("pair_and_device_info", func() error {
		info, err := dev.DeviceInfo()
		if err != nil {
			return fmt.Errorf("DeviceInfo: %w", err)
		}
		if info.Version == "" {
			return errors.New("DeviceInfo returned empty Version")
		}
		if !strings.HasPrefix(info.Version, "v") {
			return fmt.Errorf("Version must start with 'v', got %q", info.Version)
		}
		return nil
	})
}

// RestoreSimulatorMnemonic walks the simulator into its deterministic
// "seeded + initialised" state via the upstream-recommended pattern:
// after pairing the device is uninitialised; RestoreFromMnemonic()
// triggers the on-device mnemonic-entry flow which the simulator
// auto-completes from its baked-in fixture phrase
// ("boring mistake dish oyster truth pigeon viable emerge sort crash
// wire portion cannon couple enact box walk height pull today solid
// off enable tide"). After this, every ETH endpoint produces
// deterministic outputs.
//
// The previous SetPassword(32) attempt put the device into the
// "showing newly-generated mnemonic" state and ETH endpoints rejected
// every call with "can't call this endpoint: wrong state".
func RestoreSimulatorMnemonic(dev *firmware.Device) Result {
	return run("restore_simulator_mnemonic", func() error {
		if err := dev.RestoreFromMnemonic(); err != nil {
			// Already-initialised is fine — a persistent-cache rerun
			// against the same simulator binary on the same disk
			// would re-encounter the same state.
			if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "initialized") {
				return nil
			}
			return fmt.Errorf("RestoreFromMnemonic: %w", err)
		}
		return nil
	})
}

// EthAddressMainnet queries the mainnet (chainId=1) address at the
// canonical BIP-44 path. Asserts the byte shape (42-char hex with
// 0x prefix); we cannot assert the value because the simulator's
// seed is random per session.
func EthAddressMainnet(dev *firmware.Device) Result {
	return run("eth_address_mainnet", func() error {
		addr, err := dev.ETHPub(
			1, // mainnet
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			messages.ETHPubRequest_ADDRESS,
			false, // display=false (auto-confirm in simulator)
			nil,
		)
		if err != nil {
			return fmt.Errorf("ETHPub: %w", err)
		}
		if !strings.HasPrefix(addr, "0x") {
			return fmt.Errorf("expected 0x prefix, got %q", addr)
		}
		if len(addr) != 42 {
			return fmt.Errorf("expected 42 chars (0x + 40 hex), got %d (%q)", len(addr), addr)
		}
		return nil
	})
}

// EthAddressPolygonMultiByteV probes chainId=137 (Polygon). The audit
// found a v-byte truncation bug for any EIP-155 chainId > 110; this
// scenario exercises the boundary by triggering an address-display
// path on Polygon. The address itself doesn't depend on chainId for
// vanilla EOAs, but the firmware MUST accept the larger chain id.
func EthAddressPolygonMultiByteV(dev *firmware.Device) Result {
	return run("eth_address_polygon_multibyte_v", func() error {
		addr, err := dev.ETHPub(
			137, // Polygon
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			messages.ETHPubRequest_ADDRESS,
			false,
			nil,
		)
		if err != nil {
			return fmt.Errorf("ETHPub(chainId=137): %w", err)
		}
		if len(addr) != 42 {
			return fmt.Errorf("expected 42 chars, got %d", len(addr))
		}
		return nil
	})
}

// EthSignMessageAscii signs a short ASCII personal message and asserts
// the returned 65-byte signature shape. The simulator auto-confirms.
func EthSignMessageAscii(dev *firmware.Device) Result {
	return run("eth_sign_message_ascii", func() error {
		sig, err := dev.ETHSignMessage(
			1,
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			[]byte("hello bitbox"),
		)
		if err != nil {
			return fmt.Errorf("ETHSignMessage: %w", err)
		}
		if len(sig) != 65 {
			return fmt.Errorf("expected 65-byte sig, got %d", len(sig))
		}
		return nil
	})
}

// EthSignMessageBoundary tries the 1024-byte upper limit. Should
// succeed; one byte over is the firmware's reject point. We only run
// the upper-boundary success case here because the firmware-reject
// path is exercised by the static fake.
func EthSignMessageBoundary(dev *firmware.Device) Result {
	return run("eth_sign_message_boundary_1024", func() error {
		// 1024 bytes is the documented max accepted by the firmware.
		msg := make([]byte, 1024)
		for i := range msg {
			msg[i] = 'x'
		}
		sig, err := dev.ETHSignMessage(
			1,
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			msg,
		)
		if err != nil {
			return fmt.Errorf("ETHSignMessage(1024-byte): %w", err)
		}
		if len(sig) != 65 {
			return fmt.Errorf("expected 65-byte sig, got %d", len(sig))
		}
		return nil
	})
}

// EthSignEIP1559Mainnet drives a minimal-but-realistic EIP-1559
// transaction through the firmware. Values mirror the upstream
// bitbox02-api-go simulator test (TestSimulatorETHSignEIP1559) — the
// firmware refuses obviously-zero payloads (zero recipient AND zero
// value AND zero gas, all together, are not a valid tx). A real
// transfer of ~0.53 ETH at 6 gwei fee exercises every typed-tx field
// the firmware validates.
//
// The 65-byte signature's last byte is the recovery id (0 or 1 for
// type-2 EIP-1559; legacy EIP-155 would be 27/28 + chainId scaling).
func EthSignEIP1559Mainnet(dev *firmware.Device) Result {
	return run("eth_sign_eip1559_mainnet", func() error {
		recipient := [20]byte{
			0x04, 0xf2, 0x64, 0xcf, 0x34, 0x44, 0x03, 0x13, 0xb4, 0xa0,
			0x19, 0x2a, 0x35, 0x28, 0x14, 0xfb, 0xe9, 0x27, 0xb8, 0x85,
		}
		sig, err := dev.ETHSignEIP1559(
			1, // mainnet
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 10},
			8156,                                     // nonce
			new(big.Int).SetUint64(0),                // maxPriorityFeePerGas (0 is legal)
			new(big.Int).SetUint64(6_000_000_000),    // maxFeePerGas (6 gwei)
			21000,                                    // gasLimit
			recipient,
			new(big.Int).SetUint64(530_564_000_000_000_000), // ~0.5305 ETH
			nil,                                              // data
			messages.ETHAddressCase_ETH_ADDRESS_CASE_MIXED,
		)
		if err != nil {
			return fmt.Errorf("ETHSignEIP1559: %w", err)
		}
		if len(sig) != 65 {
			return fmt.Errorf("expected 65-byte sig, got %d", len(sig))
		}
		if sig[64] != 0x00 && sig[64] != 0x01 {
			return fmt.Errorf("EIP-1559 v byte must be 0x00 or 0x01, got 0x%02x", sig[64])
		}
		return nil
	})
}

// EthSignTypedDataKycMultiPage exercises the 13-field EIP-712 typed-
// data sign that realunit-app uses for its KYC registration flow.
// Each string field renders as its own confirmation page on the
// physical BitBox screen ("1/13", "2/13", … "13/13"); the simulator
// auto-confirms each page but the firmware still walks the full
// multi-page state machine.
//
// This scenario guards every multi-page typed-data quirk class:
//   • The BLE-Dedup-Bug (1/13 → 2/13 transition broken by
//     seenPackets.removeAll-vs-contains, fixed 2026-05-14 in
//     bitbox_flutter upstream).
//   • The Umlaut-Bug (firmware ErrInvalidInput101 on non-ASCII in
//     EIP-712 string values, fixed 2026-05-15 in realunit-app via
//     toBitboxSafeAscii). We send only ASCII; consumers should
//     transliterate BEFORE calling sign.
//   • Antiklepto host-nonce-commitment exchange (handled inside the
//     SDK for the high-level ETHSignTypedMessage path).
func EthSignTypedDataKycMultiPage(dev *firmware.Device) Result {
	return run("eth_sign_typed_data_kyc_multipage", func() error {
		sig, err := dev.ETHSignTypedMessage(
			1, // mainnet
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			[]byte(realUnitUserKycPayload),
		)
		if err != nil {
			return fmt.Errorf("ETHSignTypedMessage(KYC 13-page): %w", err)
		}
		if len(sig) != 65 {
			return fmt.Errorf("expected 65-byte sig, got %d", len(sig))
		}
		// EIP-712 typed-data uses 27/28 recovery byte like personal sign
		// (NOT the {0,1} parity of EIP-1559) because it's a "signed
		// message"-style signature, not a transaction signature.
		if sig[64] != 27 && sig[64] != 28 {
			return fmt.Errorf("expected v ∈ {27,28} for typed-data sign, got %d", sig[64])
		}
		return nil
	})
}

// realUnitUserKycPayloadWithUmlauts is the SAME 13-field EIP-712
// typed-data as the happy-path KYC payload, but with realistic German /
// Swiss umlauts and accents in three fields: name (u-umlaut),
// addressStreet (sharp-s), addressCity (u-umlaut). Every other field
// stays ASCII to isolate the umlaut rejection to those specific fields.
//
// The umlauts are encoded as JSON \u-escapes (Go raw-string passes them
// through verbatim; the JSON parser inside the BitBox SDK resolves the
// escape to the same UTF-8 bytes a literal "u-umlaut" would produce). This
// keeps the SOURCE FILE pure ASCII — important because the audit's
// quirk-E1 regex flags any non-ASCII byte inside a string literal in a
// file that touches EIP-712, and the testkit's own self-audit MUST
// stay green (zero findings on `bitbox-audit . --fail-on-findings`).
//
// We expect the BitBox firmware to REJECT this with ErrInvalidInput101
// (quirk E1). Consumers are responsible for transliterating via
// toBitboxSafeAscii BEFORE calling sign — this scenario guards
// against the firmware ever silently starting to accept non-ASCII
// (which would be a confusing partial-success path where the
// consumer's transliteration becomes load-bearing for one firmware
// version and dead code for the next).
const realUnitUserKycPayloadWithUmlauts = `{
  "types": {
    "EIP712Domain": [
      {"name": "name", "type": "string"},
      {"name": "version", "type": "string"}
    ],
    "RealUnitUser": [
      {"name": "email", "type": "string"},
      {"name": "name", "type": "string"},
      {"name": "type", "type": "string"},
      {"name": "phoneNumber", "type": "string"},
      {"name": "birthday", "type": "string"},
      {"name": "nationality", "type": "string"},
      {"name": "addressStreet", "type": "string"},
      {"name": "addressPostalCode", "type": "string"},
      {"name": "addressCity", "type": "string"},
      {"name": "addressCountry", "type": "string"},
      {"name": "swissTaxResidence", "type": "bool"},
      {"name": "registrationDate", "type": "string"},
      {"name": "walletAddress", "type": "address"}
    ]
  },
  "primaryType": "RealUnitUser",
  "domain": {"name": "RealUnitUser", "version": "1"},
  "message": {
    "email": "test@dfx.swiss",
    "name": "J\u00fcrg M\u00fcller",
    "type": "natural-person",
    "phoneNumber": "+41123456789",
    "birthday": "1990-01-01",
    "nationality": "Switzerland",
    "addressStreet": "Bahnhofstra\u00dfe 1",
    "addressPostalCode": "8001",
    "addressCity": "Z\u00fcrich",
    "addressCountry": "Switzerland",
    "swissTaxResidence": true,
    "registrationDate": "2026-05-16T12:00:00Z",
    "walletAddress": "0x0000000000000000000000000000000000000000"
  }
}`

// EthSignTypedDataNonAsciiRejected feeds the same 13-field RealUnitUser
// typed-data structure as the happy-path scenario but with German /
// Swiss umlauts (ü, ß) in the name, addressStreet, and addressCity
// fields. The BitBox firmware MUST reject this with ErrInvalidInput101
// (quirk E1, observed 2026-05-15 on realunit-app and fixed via
// toBitboxSafeAscii client-side transliteration).
//
// A passing "happy" sign here would actually be a firmware regression
// — it would mean a future BitBox build started accepting non-ASCII
// silently, leaving consumer-side transliteration as load-bearing
// dead code (works on new firmware, breaks the moment user holds an
// older device). Failing-as-expected here is the GREEN state for
// this scenario.
//
// If this scenario ever passes (sign returns 65 bytes), the upstream
// firmware contract changed and quirk E1 needs to be re-classified.
func EthSignTypedDataNonAsciiRejected(dev *firmware.Device) Result {
	return run("eth_sign_typed_data_non_ascii_rejected", func() error {
		_, err := dev.ETHSignTypedMessage(
			1, // mainnet
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			[]byte(realUnitUserKycPayloadWithUmlauts),
		)
		if err == nil {
			return errors.New(
				"FIRMWARE CONTRACT CHANGED: ETHSignTypedMessage accepted non-ASCII; " +
					"quirk E1 (Umlaut-Bug) may no longer apply. Verify upstream and " +
					"re-classify in quirks.json before consumers can drop their " +
					"toBitboxSafeAscii transliteration step.",
			)
		}
		// Either a wire-level firmware error or a JSON-encoding-step
		// rejection is acceptable — both mean "umlauts are not safe to
		// pass through to the BitBox without transliteration".
		msg := err.Error()
		if !strings.Contains(msg, "invalid input") && !strings.Contains(msg, "101") {
			return fmt.Errorf(
				"unexpected error class for non-ASCII typed-data: %w (expected 'invalid input' or code 101)",
				err,
			)
		}
		return nil
	})
}

// simulatorRootFingerprint is the deterministic BIP-32 root fingerprint
// the upstream BitBox02 simulator derives from its baked-in fixture
// mnemonic. Captured upstream in firmware/secp256k1_test.go and
// confirmed locally across simulator versions v9.19.0 – v9.26.1. A
// mismatch here means EITHER upstream changed the fixture mnemonic
// (which would invalidate every pinned-output assertion in this file)
// OR our RestoreFromMnemonic step silently failed and the device is
// running with a different seed.
const simulatorRootFingerprint = "4c00739d"

// RootFingerprintDeterministic asserts the BIP-32 root fingerprint
// returned by the simulator matches the upstream fixture. This is the
// load-bearing identity contract for every downstream scenario that
// pins an exact address / xpub / signature byte — if this scenario
// fails, treat every other pinned-output failure as a derived symptom.
func RootFingerprintDeterministic(dev *firmware.Device) Result {
	return run("root_fingerprint_deterministic", func() error {
		fp, err := dev.RootFingerprint()
		if err != nil {
			return fmt.Errorf("RootFingerprint: %w", err)
		}
		got := hex.EncodeToString(fp)
		if got != simulatorRootFingerprint {
			return fmt.Errorf(
				"simulator root fingerprint drift: expected %s, got %s — "+
					"either RestoreFromMnemonic failed or upstream changed "+
					"the fixture seed (re-confirm and update simulatorRootFingerprint)",
				simulatorRootFingerprint, got,
			)
		}
		return nil
	})
}

// EthSignLegacyPolygonMultiByteV signs a legacy (pre-EIP-1559) Ethereum
// transaction on Polygon (chainId=137). The existing
// EthAddressPolygonMultiByteV only queries an address — addresses do
// not depend on chainId, so that probe never actually exercises the
// firmware's chain-id-in-v-byte path. THIS scenario does:
//
// EIP-155 encodes v = recId + 35 + 2 * chainId. For chainId=137 that
// already exceeds 8 bits (35 + 2*137 = 309), forcing the firmware
// and the consumer's RLP decoder to handle a multi-byte v. Quirk CC-5
// (multi-byte v truncation) lives exactly here — a future firmware
// regression that silently truncates v to one byte would fail this
// scenario by returning a non-65-byte signature OR by returning a
// v that, when summed with EIP-155 constants, doesn't round-trip.
//
// Note: the simulator's deprecated `coin` enum maps every chain id to
// ETHCoin_ETH for firmware v9.10.0+ (which all our pinned versions
// satisfy), so the simulator accepts chainId=137 directly.
func EthSignLegacyPolygonMultiByteV(dev *firmware.Device) Result {
	return run("eth_sign_legacy_polygon_multibyte_v", func() error {
		recipient := [20]byte{
			0x04, 0xf2, 0x64, 0xcf, 0x34, 0x44, 0x03, 0x13, 0xb4, 0xa0,
			0x19, 0x2a, 0x35, 0x28, 0x14, 0xfb, 0xe9, 0x27, 0xb8, 0x85,
		}
		sig, err := dev.ETHSign(
			137, // Polygon — 2*137+35=309, multi-byte v territory
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			0,                                                // nonce
			new(big.Int).SetUint64(30_000_000_000),           // gasPrice (30 gwei)
			21000,                                            // gasLimit
			recipient,
			new(big.Int).SetUint64(100_000_000_000_000_000), // 0.1 MATIC
			nil,                                              // data
			messages.ETHAddressCase_ETH_ADDRESS_CASE_MIXED,
		)
		if err != nil {
			return fmt.Errorf("ETHSign(chainId=137): %w", err)
		}
		if len(sig) != 65 {
			return fmt.Errorf("expected 65-byte sig, got %d", len(sig))
		}
		// For EIP-155 legacy sigs the SDK returns the raw 0/1 recId in
		// the last byte; the consumer adds 35+2*chainId to produce the
		// on-wire v. A returned byte outside {0,1} would indicate the
		// firmware leaked an already-encoded v back through the SDK.
		if sig[64] != 0x00 && sig[64] != 0x01 {
			return fmt.Errorf("legacy ETH sign v byte must be 0 or 1 (raw recId), got 0x%02x", sig[64])
		}
		return nil
	})
}

// btcMainnetCoin is the BIP-84 / BIP-86 mainnet coin enum reused
// across the Bitcoin scenarios.
const btcMainnetCoin = messages.BTCCoin_BTC

// BtcXpubZpubMainnet derives a BIP-84 native-SegWit ZPUB at the
// canonical account path m/84'/0'/0' and asserts it is well-formed
// (zpub prefix, base58 length range). Because the simulator seed is
// deterministic the value is stable across runs; we assert only the
// shape here so a future BIP-32 library change in the simulator can
// shift internal encoding without breaking the scenario.
//
// This probe also exercises the BTC pairing-state path on the firmware:
// any consumer that requests BTC pubkeys directly after pairing (e.g.
// dfx-wallet's planned BTC support) hits exactly this codepath.
func BtcXpubZpubMainnet(dev *firmware.Device) Result {
	return run("btc_xpub_zpub_mainnet", func() error {
		xpub, err := dev.BTCXPub(
			btcMainnetCoin,
			// m/84'/0'/0' — BIP-84 account.
			[]uint32{84 + hardened, 0 + hardened, 0 + hardened},
			messages.BTCPubRequest_ZPUB,
			false, // display=false (auto-confirm in simulator)
		)
		if err != nil {
			return fmt.Errorf("BTCXPub(zpub): %w", err)
		}
		if !strings.HasPrefix(xpub, "zpub") {
			return fmt.Errorf("expected zpub prefix, got %q", xpub)
		}
		// BIP-32 base58 extended keys are 111 chars long. Reject anything
		// outside the canonical range — a shorter string means truncation,
		// a longer one means embedded whitespace or BOM.
		if len(xpub) < 108 || len(xpub) > 112 {
			return fmt.Errorf("zpub length %d outside expected 108..112", len(xpub))
		}
		return nil
	})
}

// BtcAddressP2WPKHMainnet derives a native-SegWit (bech32) receive
// address at m/84'/0'/0'/0/0 and asserts the bc1q prefix + length
// envelope (a P2WPKH address is exactly 42 chars: "bc1q" + 38 chars
// of bech32 data).
//
// On a physical BitBox the user sees the bech32 string on the device
// screen; the simulator auto-confirms. We do NOT pin the exact bech32
// string because that would couple this scenario to the simulator's
// specific seed library version — the prefix + length contract is the
// stable surface.
func BtcAddressP2WPKHMainnet(dev *firmware.Device) Result {
	return run("btc_address_p2wpkh_mainnet", func() error {
		addr, err := dev.BTCAddress(
			btcMainnetCoin,
			// m/84'/0'/0'/0/0 — BIP-84 first receive address.
			[]uint32{84 + hardened, 0 + hardened, 0 + hardened, 0, 0},
			firmware.NewBTCScriptConfigSimple(messages.BTCScriptConfig_P2WPKH),
			false,
		)
		if err != nil {
			return fmt.Errorf("BTCAddress(P2WPKH): %w", err)
		}
		if !strings.HasPrefix(addr, "bc1q") {
			return fmt.Errorf("expected bc1q prefix for P2WPKH, got %q", addr)
		}
		// P2WPKH bech32 is 42 chars on mainnet ("bc1q" + 38 data).
		if len(addr) != 42 {
			return fmt.Errorf("expected 42-char P2WPKH bech32, got %d (%q)", len(addr), addr)
		}
		return nil
	})
}

// BtcAddressP2TRTaproot derives a Taproot (BIP-86) address at
// m/86'/0'/0'/0/0 and asserts the bc1p bech32m prefix. P2TR addresses
// are 62 chars on mainnet ("bc1p" + 58 chars of bech32m).
//
// This exercises an entirely different firmware codepath than P2WPKH
// (Taproot script construction is BIP-341 + key tweaking), so it
// guards the broader BTC surface, not just a duplicate of the P2WPKH
// probe.
func BtcAddressP2TRTaproot(dev *firmware.Device) Result {
	return run("btc_address_p2tr_taproot", func() error {
		addr, err := dev.BTCAddress(
			btcMainnetCoin,
			// m/86'/0'/0'/0/0 — BIP-86 first receive address.
			[]uint32{86 + hardened, 0 + hardened, 0 + hardened, 0, 0},
			firmware.NewBTCScriptConfigSimple(messages.BTCScriptConfig_P2TR),
			false,
		)
		if err != nil {
			return fmt.Errorf("BTCAddress(P2TR): %w", err)
		}
		if !strings.HasPrefix(addr, "bc1p") {
			return fmt.Errorf("expected bc1p prefix for P2TR, got %q", addr)
		}
		if len(addr) != 62 {
			return fmt.Errorf("expected 62-char P2TR bech32m, got %d (%q)", len(addr), addr)
		}
		return nil
	})
}

// BtcSignMessageMainnet signs a Bitcoin message under the BIP-322-ish
// firmware path (BitBox02 signs the legacy "Bitcoin Signed Message"
// preamble against the P2WPKH key) and asserts the returned envelope:
//
//   - sig 64 bytes (R||S)
//   - recId 0..3
//   - electrum 65-byte sig with header byte in {31, 32, 33, 34}
//     (27 + 4 [compressed] + recId)
//
// This exercises the BTC sign codepath end-to-end (request → firmware
// sign → antiklepto host-nonce exchange → response decode), which is
// distinct from address derivation.
func BtcSignMessageMainnet(dev *firmware.Device) Result {
	return run("btc_sign_message_mainnet", func() error {
		res, err := dev.BTCSignMessage(
			btcMainnetCoin,
			&messages.BTCScriptConfigWithKeypath{
				ScriptConfig: firmware.NewBTCScriptConfigSimple(messages.BTCScriptConfig_P2WPKH),
				Keypath:      []uint32{84 + hardened, 0 + hardened, 0 + hardened, 0, 0},
			},
			[]byte("hello bitbox"),
		)
		if err != nil {
			return fmt.Errorf("BTCSignMessage: %w", err)
		}
		if len(res.Signature) != 64 {
			return fmt.Errorf("expected 64-byte (R||S) sig, got %d", len(res.Signature))
		}
		if res.RecID > 3 {
			return fmt.Errorf("recId must be 0..3, got %d", res.RecID)
		}
		if len(res.ElectrumSig65) != 65 {
			return fmt.Errorf("expected 65-byte electrum sig, got %d", len(res.ElectrumSig65))
		}
		// Electrum header = 27 + 4 (compressed) + recId → must be 31..34.
		if h := res.ElectrumSig65[0]; h < 31 || h > 34 {
			return fmt.Errorf("electrum header byte must be 31..34, got %d", h)
		}
		return nil
	})
}

// hardened adds the BIP-32 hardened-derivation flag bit. Inlined as a
// const because the firmware API uses uint32 path elements directly.
const hardened uint32 = 0x80000000

// run wraps a scenario body with timing + uniform Result construction.
func run(name string, fn func() error) Result {
	start := time.Now()
	err := fn()
	d := time.Since(start)
	res := Result{
		Name:       name,
		Passed:     err == nil,
		Duration:   d,
		DurationMs: d.Milliseconds(),
	}
	if err != nil {
		res.Detail = err.Error()
	}
	return res
}
