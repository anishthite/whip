package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const whipUsage = "usage: /whip up <prompt>"

func isWhipCommand(text string) bool {
	fields := strings.Fields(text)
	return len(fields) > 0 && fields[0] == "/whip"
}

// whipCommand starts an authored turn with the prompt following `/whip up`.
func (m *model) whipCommand(text string) (tea.Model, tea.Cmd) {
	prompt, ok := whipPrompt(text)
	if !ok {
		m.append(errStyle.Render(whipUsage))
		return m, nil
	}
	return m.submit(prompt)
}

// whipPrompt extracts the prompt from a `/whip up` command.
func whipPrompt(text string) (string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "/whip"))
	fields := strings.Fields(rest)
	if len(fields) < 2 || fields[0] != "up" {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(rest, "up")), true
}
