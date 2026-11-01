package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRevocationChild(t *testing.T) {
	if os.Getenv("SPACE_REVOCATION_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv("SPACE_REVOCATION_ARGS")), &args); err != nil {
		t.Fatal(err)
	}
	flag.CommandLine = flag.NewFlagSet("revocations", flag.ExitOnError)
	os.Args = append([]string{"revocations"}, args...)
	main()
}

func TestRevocationsPreserveExpiryAndExcludeMaintenance(t *testing.T) {
	source := miniredis.RunT(t)
	dest := miniredis.RunT(t)
	source.Set("revoked:test-token", "1")
	source.SetTTL("revoked:test-token", time.Hour)
	source.Set("maintenance:enabled", "true")
	source.Set("email_otp:test", "123456")
	dest.Set("maintenance:enabled", "false")
	path := filepath.Join(t.TempDir(), "revocations.json")
	run := func(server *miniredis.Miniredis, args ...string) {
		t.Helper()
		b, _ := json.Marshal(args)
		c := exec.Command(os.Args[0], "-test.run=^TestRevocationChild$")
		c.Env = append(os.Environ(), "SPACE_REVOCATION_CHILD=1", "SPACE_REVOCATION_ARGS="+string(b), "REDIS_URL=redis://"+server.Addr())
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("child failed: %s %v", out, err)
		}
	}
	run(source, "--export", path)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe export permissions")
	}
	run(dest, "--import", path)
	if dest.Exists("revoked:test-token") {
		t.Fatal("dry run wrote data")
	}
	// Simulate transfer time; imports must not extend the exported absolute expiry.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []record
	if err = json.Unmarshal(b, &entries); err != nil || len(entries) != 1 {
		t.Fatal("wrong export contents")
	}
	entries[0].Expires = time.Now().Add(30 * time.Second).UnixMilli()
	entries = append(entries, record{Key: "revoked:expired-token", Expires: time.Now().Add(-time.Second).UnixMilli()})
	b, _ = json.Marshal(entries)
	os.WriteFile(path, b, 0600)
	run(dest, "--import", path, "--apply")
	if !dest.Exists("revoked:test-token") || dest.Exists("revoked:expired-token") || dest.Exists("email_otp:test") {
		t.Fatal("wrong imported keys")
	}
	if ttl := dest.TTL("revoked:test-token"); ttl <= 0 || ttl > 30*time.Second {
		t.Fatal("expiry extended")
	}
	if value, _ := dest.Get("maintenance:enabled"); value != "false" {
		t.Fatal("maintenance was copied")
	}
	dest.SetTTL("revoked:test-token", time.Hour)
	run(dest, "--import", path, "--apply")
	if dest.TTL("revoked:test-token") < time.Hour {
		t.Fatal("existing revocation shortened")
	}
}
