// Package simulator runs the official BitBox02 Linux simulator binary and
// returns a ready-to-use firmware.Communication.
//
// The simulator is Linux/amd64 only. On other platforms Launch reports
// ErrUnsupportedPlatform and tests should t.Skip.
package simulator

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/BitBoxSwiss/bitbox02-api-go/api/firmware"
	"github.com/BitBoxSwiss/bitbox02-api-go/communication/u2fhid"
	coresim "github.com/joshuakrueger-dfx/bitbox-testkit/go/core/simulator"
)

// ErrUnsupportedPlatform indicates the host cannot run the BitBox02
// simulator binary.
var ErrUnsupportedPlatform = errors.New("bitbox/simulator: requires linux/amd64")

// Port is the TCP port the BitBox02 simulator listens on once started.
const Port = 15423

// bitboxCMD is the U2FHID command byte used by the BitBox02 over its
// non-HID transports (TCP simulator and BLE). 0x80 (cont bit) + 0x40 + 0x01.
const bitboxCMD = 0xC1

// Simulators returns the embedded list of BitBox02 simulator binaries the
// testkit knows about, sorted newest-first. Mirrors upstream's
// api/firmware/testdata/simulators.json.
//
// Override the list at runtime by setting the BITBOX_SIMULATOR env var to
// an absolute path; Launch will use that instead.
func Simulators() []coresim.Binary {
	out := make([]coresim.Binary, len(embedded))
	copy(out, embedded)
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// Instance is a running BitBox02 simulator with an attached client.
type Instance struct {
	Process *coresim.Process
	Conn    net.Conn
	Comm    firmware.Communication
}

// Stop tears down the connection and kills the simulator subprocess.
func (i *Instance) Stop() {
	if i.Conn != nil {
		_ = i.Conn.Close()
	}
	if i.Process != nil {
		_ = i.Process.Stop()
	}
}

// Launch downloads (if needed) and starts the newest known simulator,
// connects via TCP, and returns an Instance ready for use with
// firmware.NewDevice.
//
// cacheDir is where downloaded binaries live; reuse it across tests to
// avoid re-downloading.
func Launch(cacheDir string) (*Instance, error) {
	return LaunchVersion(cacheDir, "")
}

// LaunchVersion is Launch with an explicit binary version. Pass the
// `Name` field of one of Simulators() (e.g. "bitbox02-multi-9.21.0")
// or an empty string for the newest known build. The BITBOX_SIMULATOR
// env override (absolute path to a binary on disk) takes precedence
// over this argument — that lets a developer drop in a local debug
// build without needing to extend the embedded list.
//
// Returns ErrSimulatorNotFound if the name does not match any
// embedded entry, which is a deliberately distinct error from
// ErrUnsupportedPlatform so the CLI's --firmware flag can give a
// helpful "did you mean…" hint.
func LaunchVersion(cacheDir, name string) (*Instance, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return nil, ErrUnsupportedPlatform
	}

	path, err := resolveBinary(cacheDir, name)
	if err != nil {
		return nil, err
	}

	proc, err := coresim.Start(path)
	if err != nil {
		return nil, fmt.Errorf("bitbox/simulator: start: %w", err)
	}

	var conn net.Conn
	dialErr := coresim.WaitFor(10*time.Second, func() error {
		c, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", Port))
		if err != nil {
			return err
		}
		conn = c
		return nil
	})
	if dialErr != nil {
		_ = proc.Stop()
		return nil, fmt.Errorf("bitbox/simulator: dial: %w", dialErr)
	}

	return &Instance{
		Process: proc,
		Conn:    conn,
		Comm:    u2fhid.NewCommunication(conn, bitboxCMD),
	}, nil
}

// ErrSimulatorNotFound is returned when LaunchVersion is given a name
// that does not appear in Simulators().
var ErrSimulatorNotFound = errors.New("bitbox/simulator: requested version not in embedded list")

func resolveBinary(cacheDir, name string) (string, error) {
	if override := os.Getenv("BITBOX_SIMULATOR"); override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("BITBOX_SIMULATOR=%s: %w", override, err)
		}
		return abs, nil
	}

	cache, err := coresim.NewCache(cacheDir)
	if err != nil {
		return "", err
	}
	bins := Simulators()
	if len(bins) == 0 {
		return "", errors.New("bitbox/simulator: no embedded simulator list")
	}
	if name == "" {
		return cache.Resolve(bins[0])
	}
	for _, b := range bins {
		if b.Name == name {
			return cache.Resolve(b)
		}
	}
	return "", fmt.Errorf("%w: %q (have: %s)", ErrSimulatorNotFound, name, listNames(bins))
}

// listNames renders the embedded names as a comma-joined string for
// error messages.
func listNames(bins []coresim.Binary) string {
	names := make([]string, len(bins))
	for i, b := range bins {
		names[i] = b.Name
	}
	return joinNames(names)
}

// joinNames is strings.Join factored out so the simulator package
// does not pull in "strings" purely for an error helper.
func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
