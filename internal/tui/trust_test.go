package tui

import (
	"os"
	"testing"

	"github.com/context-labs/loopy/internal/config"
)

// The startup gate: a trusted cwd passes without a prompt; an untrusted one
// declines when there's no terminal to ask on.
func TestTrustGate(t *testing.T) {
	t.Setenv("LOOPY_HOME", t.TempDir())
	wd, _ := os.Getwd()
	if err := config.Trust(wd); err != nil {
		t.Fatal(err)
	}
	ok, err := checkTrust()
	if err != nil || !ok {
		t.Fatalf("trusted cwd should pass: %v %v", ok, err)
	}
}

// With the prompt disabled, an untrusted cwd declines without asking and
// reports why.
func TestTrustGatePromptDisabled(t *testing.T) {
	t.Setenv("LOOPY_HOME", t.TempDir())
	if err := config.DisableTrustPrompt(); err != nil {
		t.Fatal(err)
	}
	ok, err := checkTrust()
	if ok {
		t.Fatal("untrusted cwd should not pass with the prompt disabled")
	}
	if err == nil {
		t.Fatal("decline should explain the prompt is disabled")
	}
}
