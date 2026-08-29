package main

import (
	"strings"
	"testing"

	"github.com/context-labs/whip/internal/llm"
)

func TestUpUsesAllArgumentsAsPrompt(t *testing.T) {
	var reqs []llm.Request
	runFixture(t, "done", &reqs)

	out, err := captureCLI(t, "", upCLI, "write", "the", "release", "notes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("stdout should stream the reply, got %q", out)
	}
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	if got := userPrompt(reqs[0]); got != "write the release notes" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestUpTreatsFlagLikeArgumentsAsPrompt(t *testing.T) {
	var reqs []llm.Request
	runFixture(t, "done", &reqs)

	if _, err := captureCLI(t, "", upCLI, "--format", "json"); err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	if got := userPrompt(reqs[0]); got != "--format json" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestUpRequiresPrompt(t *testing.T) {
	err := upCLI(nil)
	if err == nil || err.Error() != upUsage {
		t.Fatalf("error = %v, want %q", err, upUsage)
	}
}

func userPrompt(req llm.Request) string {
	for _, message := range req.Messages {
		if message.Role == "user" {
			return message.Content
		}
	}
	return ""
}
