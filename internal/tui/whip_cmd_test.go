package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWhipCommandSubmitsPrompt(t *testing.T) {
	m := compactCmdModel()
	m.agent.Client = stubLLM()

	m.command("/whip up write the release notes\nand run the checks")

	const prompt = "write the release notes\nand run the checks"
	if !hasUserMsg(t, m, prompt) {
		t.Fatalf("whip command should submit only its prompt, got %+v", m.agent.MessagesSnapshot())
	}
	if hasUserMsg(t, m, "/whip up "+prompt) {
		t.Fatal("whip command must not send its prefix to the model")
	}
}

func TestWhipCommandRejectsInvalidUsage(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing action", input: "/whip"},
		{name: "missing prompt", input: "/whip up"},
		{name: "unknown action", input: "/whip down draft a plan"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := compactCmdModel()

			m.command(test.input)

			if m.busy {
				t.Fatal("invalid whip command must not start a turn")
			}
			if got := lastBlock(m); !strings.Contains(got, whipUsage) {
				t.Fatalf("usage message = %q, want %q", got, whipUsage)
			}
		})
	}
}

func TestWhipCommandQueuesPromptWhileBusy(t *testing.T) {
	m := busyQueueModel()
	m.input.SetValue("/whip up inspect the deployment logs")

	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.queue) != 1 || m.queue[0] != "inspect the deployment logs" {
		t.Fatalf("queue = %q, want stripped prompt", m.queue)
	}
}
