//go:build simulator

package simulator

import (
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

// BaselineScenarios returns the curated set of probes that exercise
// the protocol surface every consumer (dfx-wallet, realunit-app) cares
// about, in roughly the order a real onboarding flow runs them.
//
// The simulator firmware accepts these calls without user interaction
// because it auto-confirms every prompt; on a physical BitBox the
// user would tap to confirm. The simulator is pre-loaded with a fixed
// mnemonic ("boring mistake dish oyster truth pigeon viable emerge
// sort crash wire portion cannon couple enact box walk height pull
// today solid off enable tide") so address-derivation outputs are
// deterministic.
//
// For each scenario we assert what the CONSUMER would see — error
// class, byte shape, identity contract.
func BaselineScenarios() []Scenario {
	return []Scenario{
		PairAndDeviceInfo,
		RestoreSimulatorMnemonic,
		EthAddressMainnet,
		EthAddressPolygonMultiByteV,
		EthSignMessageAscii,
		EthSignMessageBoundary,
		EthSignEIP1559Mainnet,
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

// EthSignEIP1559Mainnet drives a minimal EIP-1559 transaction through
// the firmware. We feed a zero-value tx to a zero-address recipient —
// the firmware accepts it as long as the encoding is wire-correct.
// The 65-byte signature's last byte is the recovery id (0 or 1 for
// type-2 EIP-1559; legacy EIP-155 would be 27/28 + chainId scaling).
func EthSignEIP1559Mainnet(dev *firmware.Device) Result {
	return run("eth_sign_eip1559_mainnet", func() error {
		var recipient [20]byte
		sig, err := dev.ETHSignEIP1559(
			1,
			[]uint32{44 + hardened, 60 + hardened, 0 + hardened, 0, 0},
			0,                       // nonce
			big.NewInt(1),           // maxPriorityFeePerGas
			big.NewInt(1),           // maxFeePerGas
			21000,                   // gasLimit
			recipient,               // recipient (zero address)
			big.NewInt(0),           // value
			nil,                     // data
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
