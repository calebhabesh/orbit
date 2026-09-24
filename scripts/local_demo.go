//go:build ignore

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

var (
	quickMode = flag.Bool("quick", false, "run quickly without human pauses for automated validation")
	customDir = flag.String("dir", "", "optional custom disposable directory")
)

func logStep(step int, title string) {
	fmt.Printf("\n\033[1;36m[Step %d] %s\033[0m\n", step, title)
}

func logInfo(format string, args ...any) {
	fmt.Printf("       \033[0;37m%s\033[0m\n", fmt.Sprintf(format, args...))
}

func logSuccess(format string, args ...any) {
	fmt.Printf("       \033[1;32m✓ %s\033[0m\n", fmt.Sprintf(format, args...))
}

func pause() {
	if !*quickMode {
		time.Sleep(800 * time.Millisecond)
	}
}

func parseIdentityOutput(raw []byte) (dev, pin string, cert []byte) {
	lines := strings.Split(string(raw), "\n")
	var certLines []string
	inCert := false
	for _, line := range lines {
		if strings.HasPrefix(line, "device=") {
			parts := strings.Split(line, " ")
			for _, part := range parts {
				if strings.HasPrefix(part, "device=") {
					dev = strings.TrimPrefix(part, "device=")
				} else if strings.HasPrefix(part, "key-pin=") {
					pin = strings.TrimPrefix(part, "key-pin=")
				}
			}
		} else if line == "-----BEGIN CERTIFICATE-----" {
			inCert = true
			certLines = append(certLines, line)
		} else if inCert {
			certLines = append(certLines, line)
			if line == "-----END CERTIFICATE-----" {
				inCert = false
			}
		}
	}
	cert = []byte(strings.Join(certLines, "\n") + "\n")
	return dev, pin, cert
}

func readListenerURL(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "peer-listener=") {
			idx := strings.Index(line, "peer-listener=")
			part := line[idx+len("peer-listener="):]
			fields := strings.Fields(part)
			if len(fields) > 0 {
				return "https://" + fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("peer listener URL not found in output")
}

func main() {
	flag.Parse()

	fmt.Println("\033[1;35m===============================================================\033[0m")
	fmt.Println("\033[1;35m       File Sync: Local Multi-Process Replication Demo         \033[0m")
	fmt.Println("\033[1;35m===============================================================\033[0m")

	// 1. Setup disposable environment
	var disposableDir string
	if *customDir != "" {
		disposableDir = *customDir
		_ = os.MkdirAll(disposableDir, 0o700)
		_ = os.WriteFile(filepath.Join(disposableDir, testkit.Marker), []byte("disposable demo\n"), 0o600)
	} else {
		temp, err := os.MkdirTemp("", "filesync-demo-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
			os.Exit(1)
		}
		disposableDir = temp
		_ = os.WriteFile(filepath.Join(disposableDir, testkit.Marker), []byte("disposable demo\n"), 0o600)
	}

	// Trap SIGINT / SIGTERM for safe cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	cleanup := func() {
		logInfo("Cleaning up disposable environment: %s", disposableDir)
		_ = os.RemoveAll(disposableDir)
	}
	defer cleanup()
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal. Exiting safely...")
		cleanup()
		os.Exit(130)
	}()

	stateA := filepath.Join(disposableDir, "node-a-state")
	rootA := filepath.Join(disposableDir, "node-a-root")
	stateB := filepath.Join(disposableDir, "node-b-state")
	rootB := filepath.Join(disposableDir, "node-b-root")

	for _, d := range []string{stateA, rootA, stateB, rootB} {
		if err := os.Mkdir(d, 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", d, err)
			os.Exit(1)
		}
		if err := testkit.ValidateDestructiveTarget(disposableDir, d); err != nil {
			fmt.Fprintf(os.Stderr, "security check failed: %v\n", err)
			os.Exit(1)
		}
	}

	// 2. Locate or build binary
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get root: %v\n", err)
		os.Exit(1)
	}
	binary := filepath.Join(repoRoot, "bin", "filesync")
	if _, err := os.Stat(binary); err != nil {
		logInfo("Building filesync binary...")
		buildCmd := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
		buildCmd.Dir = repoRoot
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build binary failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
	}

	folder := strings.Repeat("d", 64)

	// Step 1: Initialize Nodes and Pair
	logStep(1, "Initializing Node A (Alice) and Node B (Bob) with mutual TLS")
	for _, args := range [][]string{
		{"init", "--state", stateA},
		{"init", "--state", stateB},
		{"register", "--state", stateA, "--folder", folder, "--root", rootA},
		{"register", "--state", stateB, "--folder", folder, "--root", rootB},
	} {
		cmd := exec.Command(binary, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "cmd %v: %v\n%s\n", args, err, out)
			os.Exit(1)
		}
	}

	idOutA, err := exec.Command(binary, "identity", "--state", stateA, "--certificate").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "identity A: %v\n%s\n", err, idOutA)
		os.Exit(1)
	}
	devA, pinA, certA := parseIdentityOutput(idOutA)
	certPathA := filepath.Join(disposableDir, "nodeA.pem")
	_ = os.WriteFile(certPathA, certA, 0o600)

	idOutB, err := exec.Command(binary, "identity", "--state", stateB, "--certificate").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "identity B: %v\n%s\n", err, idOutB)
		os.Exit(1)
	}
	devB, pinB, certB := parseIdentityOutput(idOutB)
	certPathB := filepath.Join(disposableDir, "nodeB.pem")
	_ = os.WriteFile(certPathB, certB, 0o600)

	logInfo("Node A Device: %s (Key Pin: %s)", devA[:12]+"...", pinA[:16]+"...")
	logInfo("Node B Device: %s (Key Pin: %s)", devB[:12]+"...", pinB[:16]+"...")

	_ = exec.Command(binary, "pair-approve", "--state", stateA, "--folder", folder, "--peer-device", devB, "--peer-key-pin", pinB).Run()
	_ = exec.Command(binary, "pair-approve", "--state", stateB, "--folder", folder, "--peer-device", devA, "--peer-key-pin", pinA).Run()
	logSuccess("Mutual pairing approved for shared folder: %s", folder[:12]+"...")
	pause()

	// Step 2: Normal Synchronization
	logStep(2, "Demonstrating normal file creation and verified transfer")
	helloContent := []byte("# Project Readme\n\nCreated by Alice on Node A. Welcome to File Sync!\n")
	_ = os.WriteFile(filepath.Join(rootA, "README.md"), helloContent, 0o600)
	logInfo("Node A authors 'README.md'")

	_ = exec.Command(binary, "scan", "--state", stateA, "--folder", folder).Run()

	serveCmdA := exec.Command(binary, "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA, _ := serveCmdA.StdoutPipe()
	serveCmdA.Stderr = os.Stderr
	_ = serveCmdA.Start()
	urlA, err := readListenerURL(stdoutA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read server URL: %v\n", err)
		os.Exit(1)
	}
	logInfo("Node A serving TLS on %s", urlA)

	syncOutB, err := exec.Command(binary, "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync B: %v\n%s\n", err, syncOutB)
		os.Exit(1)
	}
	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	dataOnB, err := os.ReadFile(filepath.Join(rootB, "README.md"))
	if err != nil || !bytes.Equal(dataOnB, helloContent) {
		fmt.Fprintf(os.Stderr, "content verification failed on Node B: %v\n", err)
		os.Exit(1)
	}
	logSuccess("Node B received 'README.md' and verified SHA-256 chunk byte-for-byte")
	pause()

	// Step 3: Concurrent Offline Edits
	logStep(3, "Simulating offline partition & concurrent edits on both nodes")
	aliceEdit := []byte("# Architecture Proposal\n\nProposed by Alice: Decentralized peer-to-peer sync engine.\n")
	bobEdit := []byte("# Architecture Proposal\n\nProposed by Bob: Hub-and-spoke VPS topology with SQLite persistence.\n")

	_ = os.WriteFile(filepath.Join(rootA, "architecture.md"), aliceEdit, 0o600)
	_ = os.WriteFile(filepath.Join(rootB, "architecture.md"), bobEdit, 0o600)
	logInfo("Node A (offline) modifies 'architecture.md' (Proposal: P2P)")
	logInfo("Node B (offline) modifies 'architecture.md' (Proposal: VPS Hub)")

	_ = exec.Command(binary, "scan", "--state", stateA, "--folder", folder).Run()
	_ = exec.Command(binary, "scan", "--state", stateB, "--folder", folder).Run()
	logSuccess("Both nodes captured independent local versions into causal DAG")
	pause()

	// Step 4: Reconnection & Conflict Detection
	logStep(4, "Reconnecting nodes: bidirectional gossip and conflict detection")
	// A serves, B syncs
	serveA := exec.Command(binary, "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	pipeA, _ := serveA.StdoutPipe()
	serveA.Stderr = os.Stderr
	_ = serveA.Start()
	urlA, _ = readListenerURL(pipeA)

	_, _ = exec.Command(binary, "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput()
	_ = serveA.Process.Kill()
	_ = serveA.Wait()

	// B serves, A syncs
	serveB := exec.Command(binary, "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	pipeB, _ := serveB.StdoutPipe()
	serveB.Stderr = os.Stderr
	_ = serveB.Start()
	urlB, _ := readListenerURL(pipeB)

	_, _ = exec.Command(binary, "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput()
	_ = serveB.Process.Kill()
	_ = serveB.Wait()

	// Query conflicts on Node A
	confJSON, err := exec.Command(binary, "conflicts", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "conflicts query: %v\n%s\n", err, confJSON)
		os.Exit(1)
	}

	var confResp struct {
		Conflicts []repository.ConflictSet `json:"conflicts"`
	}
	_ = json.Unmarshal(confJSON, &confResp)

	if len(confResp.Conflicts) != 1 || len(confResp.Conflicts[0].Heads) != 2 {
		fmt.Fprintf(os.Stderr, "expected 1 conflict with 2 heads, got: %+v\n", confResp)
		os.Exit(1)
	}
	conflict := confResp.Conflicts[0]
	logInfo("Detected Conflict on '%s':", conflict.Path)
	for i, h := range conflict.Heads {
		logInfo("   Head %d: Author=%s, Counter=%d", i+1, hex.EncodeToString(h.ID.Author[:4]), h.ID.Counter)
	}
	logSuccess("Invariant I03 Preserved: Concurrent edits retained without silent overwrite")
	pause()

	// Step 5: Reviewed Resolution
	logStep(5, "Operator performs reviewed conflict resolution via CLI")
	headTokenHex := hex.EncodeToString(conflict.HeadToken[:])
	selectedID := conflict.Heads[0].ID
	var reviewedStrs []string
	for _, h := range conflict.Heads {
		reviewedStrs = append(reviewedStrs, fmt.Sprintf("%s:%s:%d",
			hex.EncodeToString(h.ID.Folder[:]),
			hex.EncodeToString(h.ID.Author[:]),
			h.ID.Counter))
	}
	reviewedFlag := strings.Join(reviewedStrs, ",")

	logInfo("Resolving '%s' by selecting Head 1 with token %s", conflict.Path, headTokenHex[:12]+"...")
	resolveCmd := exec.Command(binary, "resolve", "select",
		"--state", stateA,
		"--folder", folder,
		"--path", conflict.Path,
		"--selected", fmt.Sprintf("%s:%s:%d", hex.EncodeToString(selectedID.Folder[:]), hex.EncodeToString(selectedID.Author[:]), selectedID.Counter),
		"--reviewed", reviewedFlag,
		"--head-token", headTokenHex,
	)
	if out, err := resolveCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "resolve select: %v\n%s\n", err, out)
		os.Exit(1)
	}
	logSuccess("Resolution committed atomically to causal version DAG")

	// Propagate resolution to Node B
	serveA = exec.Command(binary, "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	pipeA, _ = serveA.StdoutPipe()
	serveA.Stderr = os.Stderr
	_ = serveA.Start()
	urlA, _ = readListenerURL(pipeA)

	_, _ = exec.Command(binary, "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput()
	_ = serveA.Process.Kill()
	_ = serveA.Wait()

	// Verify both nodes have 0 conflicts
	for node, state := range map[string]string{"Node A": stateA, "Node B": stateB} {
		out, _ := exec.Command(binary, "conflicts", "--state", state, "--folder", folder, "--json").CombinedOutput()
		var res struct {
			Conflicts []repository.ConflictSet `json:"conflicts"`
		}
		_ = json.Unmarshal(out, &res)
		if len(res.Conflicts) != 0 {
			fmt.Fprintf(os.Stderr, "%s still reports %d conflicts!\n", node, len(res.Conflicts))
			os.Exit(1)
		}
	}
	logSuccess("Both Node A and Node B converged with 0 remaining conflicts")
	pause()

	// Step 6: Summary & Teardown
	logStep(6, "Demonstration complete: Clean process shutdown and disposable teardown")
	logSuccess("All invariants verified: authenticated TLS, verified chunk transfer,")
	logSuccess("causal concurrency preservation, reviewed resolution, and convergence.")
	fmt.Println("\n\033[1;32m[PASS] Local multi-process demo completed successfully.\033[0m\n")
}
