// Package simulator helpers for bringing a launched simulator instance
// up to a "ready for scenarios" firmware.Device.
//
// Extracted from cmd/bitbox-simulator-check so the integration test, the
// CLI, and any future consumer share the exact same Noise XX + channel-
// hash auto-acknowledgment dance. A subtle change here (e.g. raising the
// wait deadline) MUST land in every consumer at once; a shared helper
// makes that mechanical.

package simulator

import (
	"errors"
	"fmt"
	"time"

	"github.com/BitBoxSwiss/bitbox02-api-go/api/firmware"
	"github.com/flynn/noise"
)

// ConnectOptions tunes the bring-up. Zero values are sensible defaults.
type ConnectOptions struct {
	// HandshakeTimeout caps the wait for the simulator firmware to mark
	// the pairing channel as device-confirmed. The simulator auto-
	// confirms within a few hundred ms; 5s gives generous CI headroom.
	HandshakeTimeout time.Duration
	// Logger lets a caller route firmware-library logs somewhere; nil
	// uses a silent logger.
	Logger firmware.Logger
}

// Connect drives the post-Launch bring-up: firmware.NewDevice → Init →
// poll ChannelHash → ChannelHashVerify. Returns a Device that is ready
// to accept every BaselineScenarios call.
//
// The simulator regenerates its app-keypair every run (no persistence),
// so we wire an in-memory ConfigInterface.
func Connect(inst *Instance, opts ConnectOptions) (*firmware.Device, error) {
	if inst == nil {
		return nil, errors.New("simulator.Connect: nil Instance")
	}
	if inst.Comm == nil {
		return nil, errors.New("simulator.Connect: Instance has no Comm")
	}
	timeout := opts.HandshakeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	logger := opts.Logger
	if logger == nil {
		logger = noopLogger{}
	}

	dev := firmware.NewDevice(
		nil, // version: query from device via OP_INFO
		nil, // product: same
		&MemoryConfig{},
		inst.Comm,
		logger,
	)
	if err := dev.Init(); err != nil {
		return nil, fmt.Errorf("firmware.Device.Init: %w", err)
	}

	// Wait for the simulator firmware to mark the pairing as device-
	// confirmed (auto-confirms within ~ms). On a physical BitBox this
	// would require the user to compare the channel hash and tap.
	deadline := time.Now().Add(timeout)
	for {
		_, verified := dev.ChannelHash()
		if verified {
			dev.ChannelHashVerify(true)
			return dev, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"firmware.Device: channel-hash never device-verified within %s — "+
					"simulator should auto-confirm", timeout,
			)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// MemoryConfig is a minimal in-memory firmware.ConfigInterface. Suitable
// for throw-away simulator runs where the noise keypair does not need
// to survive the process.
type MemoryConfig struct {
	devicePubkeys [][]byte
	appKey        *noise.DHKey
}

// ContainsDeviceStaticPubkey returns true if pubkey was previously added.
func (c *MemoryConfig) ContainsDeviceStaticPubkey(pubkey []byte) bool {
	for _, k := range c.devicePubkeys {
		if string(k) == string(pubkey) {
			return true
		}
	}
	return false
}

// AddDeviceStaticPubkey records pubkey as trusted.
func (c *MemoryConfig) AddDeviceStaticPubkey(pubkey []byte) error {
	c.devicePubkeys = append(c.devicePubkeys, append([]byte(nil), pubkey...))
	return nil
}

// GetAppNoiseStaticKeypair returns the persisted app keypair, or nil.
func (c *MemoryConfig) GetAppNoiseStaticKeypair() *noise.DHKey { return c.appKey }

// SetAppNoiseStaticKeypair persists the app keypair.
func (c *MemoryConfig) SetAppNoiseStaticKeypair(key *noise.DHKey) error {
	c.appKey = key
	return nil
}

// noopLogger silences the firmware library's logging output.
type noopLogger struct{}

func (noopLogger) Error(string, error) {}
func (noopLogger) Info(string)         {}
func (noopLogger) Debug(string)        {}
