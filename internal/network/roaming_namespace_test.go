package network

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestWANW11IsolatedAddressDefaultRouteDetection(t *testing.T) {
	if os.Getenv("ORBIT_W11_NAMESPACE") != "isolated-marked-namespace" {
		t.Skip("requires marked disposable namespace runner")
	}
	root := os.Getenv("TMPDIR")
	if root == "" {
		t.Fatal("missing disposable root")
	}
	marker, err := os.Lstat(filepath.Join(root, testkit.Marker))
	if err != nil || !marker.Mode().IsRegular() {
		t.Fatal("missing regular disposable marker", err)
	}
	current, err := os.Readlink("/proc/self/ns/net")
	if err != nil || current == os.Getenv("ORBIT_W11_PARENT_NETNS") || os.Getenv("ORBIT_W11_PARENT_NETNS") == "" {
		t.Fatal("refusing parent namespace", err)
	}

	canonical, pathErr := filepath.EvalSymlinks(root)
	rootInfo, rootErr := os.Stat(root)
	if pathErr != nil || rootErr != nil || canonical != root || !filepath.IsAbs(root) || rootInfo.Mode().Perm()&0077 != 0 {
		t.Fatal("disposable root is not canonical/private")
	}
	uidMap, mapErr := os.ReadFile("/proc/self/uid_map")
	mapping := strings.Fields(string(uidMap))
	if os.Geteuid() != 0 || mapErr != nil || len(mapping) < 3 || (mapping[0] == "0" && mapping[1] == "0") {
		t.Fatal("requires remapped disposable user namespace")
	}
	netFD, openErr := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer unix.Close(netFD)
	ownerFD, ownerErr := unix.IoctlRetInt(netFD, 0xb701) // NS_GET_USERNS
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	defer unix.Close(ownerFD)
	var owner unix.Stat_t
	userInfo, userErr := os.Stat("/proc/self/ns/user")
	if statErr := unix.Fstat(ownerFD, &owner); statErr != nil || userErr != nil || owner.Ino != userInfo.Sys().(*syscall.Stat_t).Ino {
		t.Fatal("network namespace is not owned by disposable user namespace")
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("ip", args...)
		if output, e := cmd.CombinedOutput(); e != nil {
			t.Fatal(args, string(output), e)
		}
	}
	initial, err := NetworkSnapshot([]string{"orbit-w11"})
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan uint64, 4)
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	stop := WatchNetwork(context.Background(), func() (string, error) { return NetworkSnapshot([]string{"orbit-w11"}) }, func() { changed <- manager.NetworkChanged() }, NetworkPollInterval)
	defer stop()
	run("addr", "del", "10.23.45.1/24", "dev", "orbit-w11")
	run("addr", "add", "10.23.45.3/24", "dev", "orbit-w11")
	started := time.Now()
	select {
	case generation := <-changed:
		if generation != 2 {
			t.Fatal(generation)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("native address change missed")
	}
	addressSnapshot, err := NetworkSnapshot([]string{"orbit-w11"})
	if err != nil || addressSnapshot == initial {
		t.Fatal("address snapshot unchanged", err)
	}
	t.Logf("actual isolated Linux address change detection and manager generation: %s", time.Since(started))
	run("route", "add", "default", "via", "10.23.45.2", "dev", "orbit-w11", "metric", "10")
	started = time.Now()
	select {
	case generation := <-changed:
		if generation != 3 {
			t.Fatal(generation)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("native default route change missed")
	}
	routeSnapshot, err := NetworkSnapshot([]string{"orbit-w11"})
	if err != nil || routeSnapshot == addressSnapshot {
		t.Fatal("default route snapshot unchanged", err)
	}
	t.Logf("actual isolated Linux default route detection and manager generation: %s", time.Since(started))
}
