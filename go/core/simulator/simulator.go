// Package simulator manages the lifecycle of vendor-provided wallet
// simulator binaries: downloading them to a local cache with SHA256
// verification, starting them as a subprocess, and tearing them down.
//
// This package is vendor-agnostic. Wallet-specific glue (which binaries to
// use, how to dial the running process) lives in bitbox/simulator,
// ledger/simulator, trezor/simulator, etc.
package simulator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Binary describes a downloadable simulator artifact.
type Binary struct {
	// Name is a human-readable label, used for cached filenames and logs
	// (e.g. "bitbox02-multi-v9.26.1-simulator1.0.0-linux-amd64").
	Name string
	// URL is the absolute download location.
	URL string
	// SHA256 is the lowercase hex-encoded expected digest.
	SHA256 string
}

// Cache is a content-addressed directory of downloaded simulator binaries.
// Repeated Resolve calls for the same Binary return immediately after the
// first download.
type Cache struct {
	Dir string

	mu sync.Mutex
}

// NewCache returns a Cache backed by dir, creating the directory if needed.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Cache{Dir: dir}, nil
}

// Resolve returns the local path to b, downloading and verifying it if not
// already cached. The returned file has the executable bit set on POSIX.
func (c *Cache) Resolve(b Binary) (string, error) {
	if b.Name == "" {
		return "", errors.New("simulator: Binary.Name is required")
	}
	if b.URL == "" {
		return "", errors.New("simulator: Binary.URL is required")
	}
	if len(b.SHA256) != 64 {
		return "", fmt.Errorf("simulator: Binary.SHA256 must be 64-char hex, got %q", b.SHA256)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	path := filepath.Join(c.Dir, b.Name)
	if ok, _ := verifyFile(path, b.SHA256); ok {
		return path, nil
	}
	if err := downloadTo(path, b.URL); err != nil {
		return "", fmt.Errorf("simulator: download %s: %w", b.URL, err)
	}
	actual, err := fileHash(path)
	if err != nil {
		return "", fmt.Errorf("simulator: hash check %s: %w", path, err)
	}
	if actual != b.SHA256 {
		_ = os.Remove(path)
		return "", fmt.Errorf(
			"simulator: hash mismatch for %s — expected %s, actual %s (upstream artefact may have been rebuilt; bump the embedded SHA after manual cross-check)",
			b.Name, b.SHA256, actual,
		)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			return "", err
		}
	}
	return path, nil
}

func verifyFile(path, expected string) (bool, error) {
	got, err := fileHash(path)
	if err != nil {
		return false, err
	}
	return got == expected, nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadTo(path, url string) error {
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	resp, err := http.Get(url)
	if err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("status %s", resp.Status)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Process is a running simulator subprocess.
type Process struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
}

// Start launches binaryPath as a subprocess. Stdout is captured and
// returned line-by-line via Stdout(); stderr is inherited from the parent.
//
// The binary is wrapped with stdbuf -oL when available, so stdout is
// line-buffered for prompt readiness detection.
func Start(binaryPath string) (*Process, error) {
	var cmd *exec.Cmd
	if _, err := exec.LookPath("stdbuf"); err == nil {
		cmd = exec.Command("stdbuf", "-oL", binaryPath)
	} else {
		cmd = exec.Command(binaryPath)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, err
	}
	return &Process{cmd: cmd, stdout: stdout}, nil
}

// Stdout returns the live stdout pipe.
func (p *Process) Stdout() io.Reader { return p.stdout }

// Stop terminates the subprocess. Idempotent.
func (p *Process) Stop() error {
	if p.cmd.Process == nil {
		return nil
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	// Wait so we don't leak a zombie. Ignore the inevitable "killed" error.
	_ = p.cmd.Wait()
	return nil
}

// WaitFor polls cond at 25ms intervals until it returns nil or d elapses.
// Used by vendor packages to wait for "simulator ready" (e.g. TCP port
// accepting connections).
func WaitFor(d time.Duration, cond func() error) error {
	deadline := time.Now().Add(d)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := cond(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("WaitFor: condition never satisfied")
	}
	return fmt.Errorf("WaitFor: %s deadline: %w", d, lastErr)
}
