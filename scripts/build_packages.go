package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
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
	PackageVersion = "1.0.0"
	PackageRelease = "1"
	PackageName    = "filesync"
	FixedTimestamp = 1790208000 // 2026-09-23T00:00:00Z for reproducible builds
)

type ArchInfo struct {
	GoArch  string
	DebArch string
	RpmArch string
	RpmNum  uint16
}

var supportedArchs = []ArchInfo{
	{
		GoArch:  "amd64",
		DebArch: "amd64",
		RpmArch: "x86_64",
		RpmNum:  1,
	},
	{
		GoArch:  "arm64",
		DebArch: "arm64",
		RpmArch: "aarch64",
		RpmNum:  19,
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "build_packages error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	distDir := filepath.Join(repoRoot, "dist")
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}

	// Embed an explicit source revision; deterministic packaging timestamps stay
	// fixed. Export FILESYNC_BUILD_COMMIT for source archives without Git metadata.
	buildCommit := os.Getenv("FILESYNC_BUILD_COMMIT")
	if buildCommit == "" {
		output, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
		if err != nil {
			return fmt.Errorf("set FILESYNC_BUILD_COMMIT when building an archive without git metadata: %w", err)
		}
		buildCommit = strings.TrimSpace(string(output))
	}
	buildDate := os.Getenv("FILESYNC_BUILD_DATE")
	if buildDate == "" {
		buildDate = "2026-10-01"
	}

	// 1. Build binaries for amd64 and arm64
	for _, arch := range supportedArchs {
		binTarget := filepath.Join(binDir, fmt.Sprintf("filesync-linux-%s", arch.GoArch))
		if arch.GoArch == "amd64" {
			binTarget = filepath.Join(binDir, "filesync")
		}

		fmt.Printf("Building binary for linux/%s -> %s\n", arch.GoArch, binTarget)
		cmd := exec.Command("go", "build", "-trimpath",
			"-ldflags", fmt.Sprintf("-X main.version=%s -X main.commit=%s -X main.date=%s", PackageVersion, buildCommit, buildDate),
			"-o", binTarget, "./cmd/filesync")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch.GoArch)
		cmd.Dir = repoRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("compile %s: %w", arch.GoArch, err)
		}
	}

	// Read common files
	serviceBytes, err := os.ReadFile(filepath.Join(repoRoot, "packaging/systemd/filesync.service"))
	if err != nil {
		return err
	}
	installScriptBytes, err := os.ReadFile(filepath.Join(repoRoot, "packaging/scripts/install.sh"))
	if err != nil {
		return err
	}
	uninstallScriptBytes, err := os.ReadFile(filepath.Join(repoRoot, "packaging/scripts/uninstall.sh"))
	if err != nil {
		return err
	}
	licenseBytes, err := os.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil {
		licenseBytes = []byte("MIT License\n")
	}
	noticeBytes, err := os.ReadFile(filepath.Join(repoRoot, "NOTICE"))
	if err != nil {
		return err
	}
	readmeBytes, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		return err
	}

	var generatedPackages []string

	for _, arch := range supportedArchs {
		binPath := filepath.Join(binDir, fmt.Sprintf("filesync-linux-%s", arch.GoArch))
		if arch.GoArch == "amd64" {
			binPath = filepath.Join(binDir, "filesync")
		}
		binBytes, err := os.ReadFile(binPath)
		if err != nil {
			return fmt.Errorf("read binary %s: %w", binPath, err)
		}

		// A. Build tar.gz package
		tarGzName := fmt.Sprintf("%s-v%s-linux-%s.tar.gz", PackageName, PackageVersion, arch.GoArch)
		tarGzPath := filepath.Join(distDir, tarGzName)
		fmt.Printf("Generating %s...\n", tarGzName)
		if err := buildTarGz(tarGzPath, binBytes, serviceBytes, installScriptBytes, uninstallScriptBytes, licenseBytes, noticeBytes, readmeBytes); err != nil {
			return fmt.Errorf("build tar.gz for %s: %w", arch.GoArch, err)
		}
		generatedPackages = append(generatedPackages, tarGzPath)

		// B. Build .deb package
		debName := fmt.Sprintf("%s_%s_%s.deb", PackageName, PackageVersion, arch.DebArch)
		debPath := filepath.Join(distDir, debName)
		fmt.Printf("Generating %s...\n", debName)
		if err := buildDeb(debPath, arch.DebArch, binBytes, serviceBytes, licenseBytes, noticeBytes); err != nil {
			return fmt.Errorf("build deb for %s: %w", arch.DebArch, err)
		}
		generatedPackages = append(generatedPackages, debPath)

		// C. Build .rpm package
		rpmName := fmt.Sprintf("%s-%s-%s.%s.rpm", PackageName, PackageVersion, PackageRelease, arch.RpmArch)
		rpmPath := filepath.Join(distDir, rpmName)
		fmt.Printf("Generating %s...\n", rpmName)
		if err := buildRpm(rpmPath, arch, binBytes, serviceBytes, licenseBytes, noticeBytes); err != nil {
			return fmt.Errorf("build rpm for %s: %w", arch.RpmArch, err)
		}
		generatedPackages = append(generatedPackages, rpmPath)
	}

	// 5. Generate SHA256SUMS
	fmt.Println("Generating SHA256SUMS...")
	sumsPath := filepath.Join(distDir, "SHA256SUMS")
	var sumsBuf bytes.Buffer
	for _, p := range generatedPackages {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		fmt.Fprintf(&sumsBuf, "%s  %s\n", hex.EncodeToString(h[:]), filepath.Base(p))
	}
	if err := os.WriteFile(sumsPath, sumsBuf.Bytes(), 0o644); err != nil {
		return err
	}

	fmt.Println("Release packaging complete! Generated artifacts:")
	for _, p := range generatedPackages {
		fi, _ := os.Stat(p)
		fmt.Printf("  - %s (%d bytes)\n", filepath.Base(p), fi.Size())
	}
	fmt.Printf("  - SHA256SUMS\n")

	return nil
}

func buildTarGz(outPath string, bin, service, installScript, uninstallScript, license, notice, readme []byte) error {
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	fixedTime := time.Unix(FixedTimestamp, 0).UTC()

	entries := []struct {
		Name string
		Mode int64
		Data []byte
	}{
		{"filesync", 0755, bin},
		{"systemd/filesync.service", 0644, service},
		{"install.sh", 0755, installScript},
		{"uninstall.sh", 0755, uninstallScript},
		{"LICENSE", 0644, license},
		{"NOTICE", 0644, notice},
		{"README.md", 0644, readme},
	}

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Size:     int64(len(e.Data)),
			ModTime:  fixedTime,
			Uid:      0,
			Gid:      0,
			Uname:    "root",
			Gname:    "root",
			Format:   tar.FormatPAX,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(e.Data); err != nil {
			return err
		}
	}
	return nil
}

func buildDeb(outPath string, debArch string, bin, service, license, notice []byte) error {
	fixedTime := time.Unix(FixedTimestamp, 0).UTC()

	// 1. control.tar.gz
	var controlBuf bytes.Buffer
	cgz := gzip.NewWriter(&controlBuf)
	ctw := tar.NewWriter(cgz)

	controlContent := fmt.Sprintf(`Package: %s
Version: %s
Section: utils
Priority: optional
Architecture: %s
Maintainer: File Sync Maintainers <maintainers@example.com>
Installed-Size: %d
Description: Distributed file synchronization agent with causal consistency
 File Sync is an autonomous background synchronization daemon providing
 SQLite-backed causal history tracking, embedded web management console,
 and crash-resilient two-phase working tree publication.
`, PackageName, PackageVersion, debArch, (len(bin)+len(service)+len(license)+len(notice))/1024)

	prermContent := `#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
    systemctl --user stop filesync.service 2>/dev/null || true
fi
exit 0
`

	postinstContent := `#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload 2>/dev/null || true
fi
exit 0
`

	postrmContent := `#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload 2>/dev/null || true
fi
# DATA PRESERVATION GUARANTEE (Invariant S21):
# Package removal strictly preserves ~/.local/share/filesync, ~/.filesync,
# and all operator workspace folder contents.
exit 0
`

	cEntries := []struct {
		Name string
		Mode int64
		Data []byte
	}{
		{"./control", 0644, []byte(controlContent)},
		{"./prerm", 0755, []byte(prermContent)},
		{"./postinst", 0755, []byte(postinstContent)},
		{"./postrm", 0755, []byte(postrmContent)},
	}

	for _, e := range cEntries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Size:     int64(len(e.Data)),
			ModTime:  fixedTime,
			Format:   tar.FormatPAX,
			Typeflag: tar.TypeReg,
		}
		if err := ctw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := ctw.Write(e.Data); err != nil {
			return err
		}
	}
	if err := ctw.Close(); err != nil {
		return err
	}
	if err := cgz.Close(); err != nil {
		return err
	}

	// 2. data.tar.gz
	var dataBuf bytes.Buffer
	dgz := gzip.NewWriter(&dataBuf)
	dtw := tar.NewWriter(dgz)

	dEntries := []struct {
		Name string
		Mode int64
		Data []byte
	}{
		{"./usr/bin/filesync", 0755, bin},
		{"./usr/lib/systemd/user/filesync.service", 0644, service},
		{"./usr/share/doc/filesync/copyright", 0644, license},
		{"./usr/share/doc/filesync/NOTICE", 0644, notice},
	}

	for _, e := range dEntries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Size:     int64(len(e.Data)),
			ModTime:  fixedTime,
			Format:   tar.FormatPAX,
			Typeflag: tar.TypeReg,
		}
		if err := dtw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := dtw.Write(e.Data); err != nil {
			return err
		}
	}
	if err := dtw.Close(); err != nil {
		return err
	}
	if err := dgz.Close(); err != nil {
		return err
	}

	// 3. Assemble ar archive
	debFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer debFile.Close()

	if _, err := debFile.WriteString("!<arch>\n"); err != nil {
		return err
	}

	writeArMember := func(name string, data []byte) error {
		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
			name, FixedTimestamp, 0, 0, 0100644, len(data))
		if _, err := debFile.WriteString(hdr); err != nil {
			return err
		}
		if _, err := debFile.Write(data); err != nil {
			return err
		}
		if len(data)%2 != 0 {
			if _, err := debFile.Write([]byte("\n")); err != nil {
				return err
			}
		}
		return nil
	}

	if err := writeArMember("debian-binary", []byte("2.0\n")); err != nil {
		return err
	}
	if err := writeArMember("control.tar.gz", controlBuf.Bytes()); err != nil {
		return err
	}
	if err := writeArMember("data.tar.gz", dataBuf.Bytes()); err != nil {
		return err
	}

	return nil
}

func buildRpm(outPath string, arch ArchInfo, bin, service, license, notice []byte) error {
	// Create CPIO payload (format "070701")
	var cpioBuf bytes.Buffer
	files := []struct {
		Path string
		Mode uint32
		Data []byte
	}{
		{"usr/bin/filesync", 0100755, bin},
		{"usr/lib/systemd/user/filesync.service", 0100644, service},
		{"usr/share/doc/filesync/LICENSE", 0100644, license},
		{"usr/share/doc/filesync/NOTICE", 0100644, notice},
	}

	for i, f := range files {
		name := f.Path
		hdr := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
			i+1, f.Mode, 0, 0, 1, FixedTimestamp, len(f.Data), 3, 1, 0, 0, len(name)+1, 0)
		cpioBuf.WriteString(hdr)
		cpioBuf.WriteString(name)
		cpioBuf.WriteByte(0)
		for cpioBuf.Len()%4 != 0 {
			cpioBuf.WriteByte(0)
		}
		cpioBuf.Write(f.Data)
		for cpioBuf.Len()%4 != 0 {
			cpioBuf.WriteByte(0)
		}
	}
	// Trailer
	trailerName := "TRAILER!!!"
	trailerHdr := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, len(trailerName)+1, 0)
	cpioBuf.WriteString(trailerHdr)
	cpioBuf.WriteString(trailerName)
	cpioBuf.WriteByte(0)
	for cpioBuf.Len()%512 != 0 {
		cpioBuf.WriteByte(0)
	}

	// Gzip CPIO
	var gzipPayload bytes.Buffer
	gw := gzip.NewWriter(&gzipPayload)
	gw.Write(cpioBuf.Bytes())
	gw.Close()

	// Build RPM file
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 1. Lead (96 bytes)
	lead := make([]byte, 96)
	copy(lead[0:4], []byte{0xed, 0xab, 0xee, 0xdb})
	lead[4] = 3
	lead[5] = 0
	binary.BigEndian.PutUint16(lead[6:8], 0) // binary package
	binary.BigEndian.PutUint16(lead[8:10], arch.RpmNum)
	nameField := fmt.Sprintf("%s-%s-%s", PackageName, PackageVersion, PackageRelease)
	copy(lead[10:76], []byte(nameField))
	binary.BigEndian.PutUint16(lead[76:78], 1) // OS: Linux
	binary.BigEndian.PutUint16(lead[78:80], 5) // SigType: RPM_SIG_HEADER
	f.Write(lead)

	// Build main header
	mainHdrBuf := buildRpmMainHeader(arch.RpmArch, files)

	// Build signature header
	sigHdrBuf := buildRpmSigHeader(uint32(mainHdrBuf.Len() + gzipPayload.Len()))

	f.Write(sigHdrBuf.Bytes())
	f.Write(mainHdrBuf.Bytes())
	f.Write(gzipPayload.Bytes())

	return nil
}

type rpmIndexEntry struct {
	Tag    uint32
	Type   uint32
	Offset uint32
	Count  uint32
}

func buildRpmSigHeader(totalSize uint32) *bytes.Buffer {
	var data bytes.Buffer
	var entries []rpmIndexEntry

	// Tag 1000: SIGSIZE (size of Header + Payload)
	offset := uint32(data.Len())
	var szBuf [4]byte
	binary.BigEndian.PutUint32(szBuf[:], totalSize)
	data.Write(szBuf[:])
	entries = append(entries, rpmIndexEntry{
		Tag:    1000,
		Type:   3, // int32
		Offset: offset,
		Count:  1,
	})

	return serializeRpmHeader(entries, data.Bytes())
}

func buildRpmMainHeader(rpmArch string, files []struct {
	Path string
	Mode uint32
	Data []byte
}) *bytes.Buffer {
	var data bytes.Buffer
	var entries []rpmIndexEntry

	addString := func(tag uint32, s string) {
		offset := uint32(data.Len())
		data.WriteString(s)
		data.WriteByte(0)
		entries = append(entries, rpmIndexEntry{
			Tag:    tag,
			Type:   6, // string
			Offset: offset,
			Count:  1,
		})
	}

	addStringArray := func(tag uint32, arr []string) {
		offset := uint32(data.Len())
		for _, s := range arr {
			data.WriteString(s)
			data.WriteByte(0)
		}
		entries = append(entries, rpmIndexEntry{
			Tag:    tag,
			Type:   8, // string array
			Offset: offset,
			Count:  uint32(len(arr)),
		})
	}

	addInt32Array := func(tag uint32, arr []uint32) {
		// align 4
		for data.Len()%4 != 0 {
			data.WriteByte(0)
		}
		offset := uint32(data.Len())
		for _, v := range arr {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], v)
			data.Write(b[:])
		}
		entries = append(entries, rpmIndexEntry{
			Tag:    tag,
			Type:   4, // int32
			Offset: offset,
			Count:  uint32(len(arr)),
		})
	}

	addInt16Array := func(tag uint32, arr []uint16) {
		for data.Len()%2 != 0 {
			data.WriteByte(0)
		}
		offset := uint32(data.Len())
		for _, v := range arr {
			var b [2]byte
			binary.BigEndian.PutUint16(b[:], v)
			data.Write(b[:])
		}
		entries = append(entries, rpmIndexEntry{
			Tag:    tag,
			Type:   3, // int16
			Offset: offset,
			Count:  uint32(len(arr)),
		})
	}

	addString(1000, PackageName)
	addString(1001, PackageVersion)
	addString(1002, PackageRelease)
	addString(1004, "Distributed file synchronization agent with causal consistency")
	addString(1005, "Autonomous background synchronization agent with SQLite metadata and embedded web console.")
	addString(1014, "MIT")
	addString(1016, "Applications/System")
	addString(1021, "linux")
	addString(1022, rpmArch)
	addString(1124, "cpio")
	addString(1125, "gzip")
	addString(1126, "6")

	// Post-uninstall script preserving user data
	addString(1085, "#!/bin/sh\n# DATA PRESERVATION GUARANTEE (Invariant S21):\n# ~/.local/share/filesync and workspace files are strictly preserved\nexit 0\n")

	// File tags
	var baseNames []string
	var dirNames []string
	var dirIndexes []uint32
	var fileSizes []uint32
	var fileModes []uint16
	var fileMD5s []string

	dirMap := make(map[string]uint32)
	for _, f := range files {
		dir := "/" + filepath.Dir(f.Path) + "/"
		base := filepath.Base(f.Path)

		dIdx, exists := dirMap[dir]
		if !exists {
			dIdx = uint32(len(dirNames))
			dirNames = append(dirNames, dir)
			dirMap[dir] = dIdx
		}

		baseNames = append(baseNames, base)
		dirIndexes = append(dirIndexes, dIdx)
		fileSizes = append(fileSizes, uint32(len(f.Data)))
		fileModes = append(fileModes, uint16(f.Mode))
		h := md5.Sum(f.Data)
		fileMD5s = append(fileMD5s, hex.EncodeToString(h[:]))
	}

	addStringArray(1117, baseNames)
	addStringArray(1118, dirNames)
	addInt32Array(1116, dirIndexes)
	addInt32Array(1028, fileSizes)
	addInt16Array(1030, fileModes)
	addStringArray(1035, fileMD5s)

	return serializeRpmHeader(entries, data.Bytes())
}

func serializeRpmHeader(entries []rpmIndexEntry, data []byte) *bytes.Buffer {
	// Sort entries by Tag as required by RPM spec
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Tag < entries[j].Tag
	})

	var buf bytes.Buffer
	buf.Write([]byte{0x8e, 0xad, 0xe8, 0x01}) // Magic
	buf.Write([]byte{0, 0, 0, 0})             // Reserved

	var nindexBuf [4]byte
	binary.BigEndian.PutUint32(nindexBuf[:], uint32(len(entries)))
	buf.Write(nindexBuf[:])

	var nbytesBuf [4]byte
	binary.BigEndian.PutUint32(nbytesBuf[:], uint32(len(data)))
	buf.Write(nbytesBuf[:])

	for _, e := range entries {
		var b [16]byte
		binary.BigEndian.PutUint32(b[0:4], e.Tag)
		binary.BigEndian.PutUint32(b[4:8], e.Type)
		binary.BigEndian.PutUint32(b[8:12], e.Offset)
		binary.BigEndian.PutUint32(b[12:16], e.Count)
		buf.Write(b[:])
	}
	buf.Write(data)

	// Pad to 8-byte boundary
	for buf.Len()%8 != 0 {
		buf.WriteByte(0)
	}

	return &buf
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("repository root containing go.mod not found")
}
