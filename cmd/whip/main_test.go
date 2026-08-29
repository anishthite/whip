package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/context-labs/whip/internal/tui"
)

// The system prompt always carries the built-in operating rules (the safety
// rails); ~/.whip/me.md appends the user's standing instructions after them.
func TestSystemPromptAppendsUserMe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WHIP_HOME", home)

	p := systemPrompt(t.TempDir())
	if !strings.Contains(p, "never force-push") {
		t.Fatal("built-in operating rules must always be present")
	}
	if strings.Contains(p, "Standing instructions") {
		t.Fatal("a fresh install (all-comments me.md) appends nothing")
	}

	os.WriteFile(filepath.Join(home, "me.md"), []byte("- Always pnpm, never npm.\n"), 0o644)
	p = systemPrompt(t.TempDir())
	if !strings.Contains(p, "never force-push") {
		t.Fatal("built-in rules survive a user me.md")
	}
	if !strings.Contains(p, "Standing instructions from the user") || !strings.Contains(p, "Always pnpm") {
		t.Fatalf("user instructions should append:\n%s", p)
	}
}

func TestSessionStart(t *testing.T) {
	tests := []struct {
		name           string
		resumeID       string
		continueRecent bool
		browse         bool
		want           tui.SessionStart
		wantErr        string
	}{
		{name: "none", want: tui.SessionStartNone},
		{name: "explicit id", resumeID: "abc", want: tui.SessionStartNone},
		{name: "continue", continueRecent: true, want: tui.SessionStartContinue},
		{name: "browse", browse: true, want: tui.SessionStartBrowse},
		{name: "continue and browse conflict", continueRecent: true, browse: true, wantErr: "choose only one"},
		{name: "id and continue conflict", resumeID: "abc", continueRecent: true, wantErr: "choose only one"},
		{name: "id and browse conflict", resumeID: "abc", browse: true, wantErr: "choose only one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sessionStart(tt.resumeID, tt.continueRecent, tt.browse)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("sessionStart error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("sessionStart = %v, want %v", got, tt.want)
			}
		})
	}
}
