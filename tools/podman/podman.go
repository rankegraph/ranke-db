// package: podman / tools
// type:    test-support
// job:     run a throwaway container for an adapter's real-counterpart test, on a free port, torn down after
// limits:  a test helper; it skips without podman and waits for the port, not for readiness
//
// Package podman is the shared boilerplate for the real-counterpart adapter tests
// (OpenBao, and lowkey-vault for the Azure backends). An adapter's only meaningful
// test drives its real backend, spun up here; Run publishes it on a free host
// port, returns that address plus a teardown, and skips the test when podman is
// missing so the offline gate stays green.
package podman

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// Spec describes a container to run for a test.
type Spec struct {
	Image string            // image reference, e.g. ghcr.io/openbao/openbao:latest
	Port  int               // container port to publish and wait for
	More  []int             // further container ports to publish, addressed through RunMore
	Env   map[string]string // environment variables
	Args  []string          // command + args after the image
}

// Run starts spec's container and returns the address of spec.Port plus a
// teardown. It waits until that port accepts connections; the caller performs any
// service-specific readiness (e.g. an unseal/health check).
func Run(t testing.TB, spec Spec) (addr string, teardown func()) {
	t.Helper()
	addr, _, teardown = RunMore(t, spec)
	return addr, teardown
}

// RunMore is Run for a counterpart whose API and token endpoint listen separately:
// it also returns spec.More's addresses, keyed by container port.
func RunMore(t testing.TB, spec Spec) (addr string, more map[int]string, teardown func()) {
	t.Helper()
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skipf("podman not found; skipping %s test", spec.Image)
	}

	port := freePort(t)
	addr = fmt.Sprintf("127.0.0.1:%d", port)
	name := fmt.Sprintf("ranke-test-%d", port)

	args := []string{"run", "--rm", "-d", "--name", name, "-p", addr + ":" + strconv.Itoa(spec.Port)}
	more = make(map[int]string, len(spec.More))
	for _, p := range spec.More {
		a := fmt.Sprintf("127.0.0.1:%d", freePort(t))
		more[p] = a
		args = append(args, "-p", a+":"+strconv.Itoa(p))
	}
	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, spec.Image)
	args = append(args, spec.Args...)
	if out, err := exec.Command("podman", args...).CombinedOutput(); err != nil {
		t.Fatalf("podman run %s: %v: %s", spec.Image, err, out)
	}
	teardown = func() { _ = exec.Command("podman", "rm", "-f", name).Run() }

	waitPort(t, addr, teardown)
	return addr, more, teardown
}

func freePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func waitPort(t testing.TB, addr string, teardown func()) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			teardown()
			t.Fatalf("container port %s did not open in time", addr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
