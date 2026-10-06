//go:build ignore

// build_orbit_net builds the operator-only orbit-net archives for each
// supported server architecture. They are separate from end-user packages:
// installing Orbit never installs or starts a connection service.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	version        = "1.0.0"
	fixedTimestamp = 1790208000 // 2026-09-23T00:00:00Z for reproducible archives
)

var archs = []string{"amd64", "arm64"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "build_orbit_net:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("run from the repository root: %w", err)
	}
	commit := os.Getenv("FILESYNC_BUILD_COMMIT")
	if commit == "" {
		out, e := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
		if e != nil {
			return fmt.Errorf("set FILESYNC_BUILD_COMMIT without git metadata: %w", e)
		}
		commit = strings.TrimSpace(string(out))
	}
	date := os.Getenv("FILESYNC_BUILD_DATE")
	if date == "" {
		date = "2026-10-01"
	}
	assets := map[string]string{
		"lib/systemd/system/orbit-net.service":              "packaging/systemd/orbit-net.service",
		"lib/sysusers.d/orbit-net.conf":                     "packaging/sysusers/orbit-net.conf",
		"share/doc/orbit-net/orbit-net-operator.md":         "docs/orbit-net-operator.md",
		"share/doc/orbit-net/serve.example.json":            "packaging/orbit-net/serve.example.json",
		"share/doc/orbit-net/profile-template.example.json": "packaging/orbit-net/profile-template.example.json",
		"share/doc/orbit-net/NOTICE":                        "NOTICE",
		"share/doc/orbit-net/LICENSES.md":                   "packaging/LICENSES.md",
	}
	// Mirrors build_packages.go: the repository may not carry a LICENSE file.
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		license = []byte("MIT License\n")
	}
	files := map[string][]byte{"share/doc/orbit-net/LICENSE": license}
	for dst, src := range assets {
		data, e := os.ReadFile(filepath.Join(root, src))
		if e != nil {
			return e
		}
		files[dst] = data
	}
	dist := filepath.Join(root, "dist")
	if err = os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	var sums []string
	for _, arch := range archs {
		bin := filepath.Join(root, "bin", "orbit-net-linux-"+arch)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", fmt.Sprintf("-X main.version=%s -X main.commit=%s -X main.date=%s", version, commit, date), "-o", bin, "./cmd/orbit-net")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("compile %s: %w", arch, err)
		}
		data, e := os.ReadFile(bin)
		if e != nil {
			return e
		}
		name := fmt.Sprintf("orbit-net-v%s-linux-%s.tar.gz", version, arch)
		archive, e := tarball(fmt.Sprintf("orbit-net-v%s-linux-%s", version, arch), data, files)
		if e != nil {
			return e
		}
		if err = os.WriteFile(filepath.Join(dist, name), archive, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(archive)
		sums = append(sums, hex.EncodeToString(sum[:])+"  "+name)
		fmt.Println("built", filepath.Join("dist", name))
	}
	return os.WriteFile(filepath.Join(dist, "orbit-net-SHA256SUMS"), []byte(strings.Join(sums, "\n")+"\n"), 0o644)
}

func tarball(prefix string, bin []byte, files map[string][]byte) ([]byte, error) {
	var out bytes.Buffer
	gz, err := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	gz.ModTime = time.Unix(fixedTimestamp, 0)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, data []byte) error {
		h := &tar.Header{Name: prefix + "/" + name, Mode: mode, Size: int64(len(data)), ModTime: time.Unix(fixedTimestamp, 0), Uid: 0, Gid: 0, Uname: "root", Gname: "root", Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	if err = add("bin/orbit-net", 0o755, bin); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err = add(name, 0o644, files[name]); err != nil {
			return nil, err
		}
	}
	if err = tw.Close(); err != nil {
		return nil, err
	}
	if err = gz.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
