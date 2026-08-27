package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustRoundTrip(t *testing.T) {
	t.Setenv("WHIP_HOME", t.TempDir())
	dir := "/home/abe/code/loopy"
	if Trusted(dir) {
		t.Fatal("should not be trusted initially")
	}
	if err := Trust(dir); err != nil {
		t.Fatal(err)
	}
	if !Trusted(dir) {
		t.Fatal("should be trusted after Trust()")
	}
	// persists across reads
	if !Trusted(dir) {
		t.Fatal("trust should persist")
	}
	// a different path is not trusted
	if Trusted("/other/path") {
		t.Fatal("unrelated path must not be trusted")
	}
	// file exists in WHIP_HOME
	home, _ := Dir()
	if _, err := os.Stat(filepath.Join(home, "trusted.json")); err != nil {
		t.Fatalf("trusted.json missing: %v", err)
	}
}

func TestTrustPromptFlag(t *testing.T) {
	t.Setenv("WHIP_HOME", t.TempDir())
	if TrustPromptDisabled() {
		t.Fatal("prompt should be enabled by default")
	}
	if err := DisableTrustPrompt(); err != nil {
		t.Fatal(err)
	}
	if !TrustPromptDisabled() {
		t.Fatal("prompt should be disabled after DisableTrustPrompt()")
	}
	// the flag survives in config.json and doesn't clobber other settings
	home, _ := Dir()
	data, _ := os.ReadFile(filepath.Join(home, "config.json"))
	var c Config
	if err := parseJSONC(data, &c); err != nil || !c.NoTrustPrompt {
		t.Fatalf("config.json should carry noTrustPrompt: %v %s", err, data)
	}
}
