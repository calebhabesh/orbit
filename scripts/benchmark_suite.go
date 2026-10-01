//go:build ignore

// Compatibility entry point. Validation runs in isolated, marked directories.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	python, err := exec.LookPath("python3")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	output := filepath.Join("docs", "evidence", "validation-"+time.Now().UTC().Format("20060102T150405Z"))
	args := append([]string{python, "scripts/validation/benchmark.py", "--output", output}, os.Args[1:]...)
	if err := syscall.Exec(python, args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
