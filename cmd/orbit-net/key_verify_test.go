package main

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestW13OfflineAuthorityRestoreVerification(t *testing.T) {
	k := newKit(t)
	original, err := os.ReadFile(k.authority)
	if err != nil {
		t.Fatal(err)
	}
	restored := k.path("restored.key")
	if err := os.WriteFile(restored, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, authority := range []string{k.authorityPub, strings.ToUpper(k.authorityPub)} {
		out, err := k.run("key", "verify", "--file", restored, "--authority", authority)
		if err != nil || out != "verified authority: "+k.authorityPub+"\n" {
			t.Fatal("restored signing capability not verified", err, out)
		}
	}
	after, _ := os.ReadFile(restored)
	if string(after) != string(original) {
		t.Fatal("verification changed restored key")
	}
	checkRejected := func(authority string) {
		t.Helper()
		out, err := k.run("key", "verify", "--file", restored, "--authority", authority)
		if err == nil || strings.Contains(out, strings.TrimSpace(string(original))) || strings.Contains(out, "verified authority:") {
			t.Fatal("invalid backup accepted or private key exposed", err, out)
		}
	}
	checkRejected(k.servicePub)
	checkRejected("invalid")
	// Retain the expected public half, but corrupt the seed: comparing only
	// PrivateKey.Public would accept this unusable backup.
	private, _ := hex.DecodeString(strings.TrimSpace(string(original)))
	private[0] ^= 1
	if err := os.WriteFile(restored, []byte(hex.EncodeToString(private)), 0600); err != nil {
		t.Fatal(err)
	}
	checkRejected(k.authorityPub)
	if err := os.WriteFile(restored, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restored, 0644); err != nil {
		t.Fatal(err)
	}
	checkRejected(k.authorityPub)
	if err := os.Remove(restored); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(k.authority, restored); err != nil {
		t.Fatal(err)
	}
	checkRejected(k.authorityPub)
}
