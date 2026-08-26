package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/context-labs/whip/internal/claudeauth"
)

func TestClaudeStreamMapsOAuthMessagesAndTools(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		for header, want := range map[string]string{
			"Authorization":     "Bearer access",
			"anthropic-version": "2023-06-01",
			"anthropic-beta":    "claude-code-20250219,oauth-2025-04-20",
			"anthropic-dangerous-direct-browser-access": "true",
			"user-agent":   "claude-cli/" + claudeCodeVersion,
			"x-app":        "cli",
			"Accept":       "text/event-stream",
			"Content-Type": "application/json",
		} {
			if got := r.Header.Get(header); got != want {
				http.Error(w, header+" = "+got, http.StatusBadRequest)
				return
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"plan\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"done\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call-1\",\"name\":\"Bash\",\"input\":{\"command\":\"pwd\"}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":12,\"output_tokens\":7,\"cache_read_input_tokens\":5}}\n\n")
	}))
	defer server.Close()

	var text, think strings.Builder
	message, usage, err := NewClaude(server.URL, claudeSource(t)).Stream(context.Background(), Request{
		Model:           "claude-sonnet-4-6",
		MaxTokens:       64000,
		ReasoningEffort: "high",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "inspect"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "old", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path":"README.md"}`}}}},
			{Role: "tool", ToolCallID: "old", Content: "contents"},
		},
		Tools: []Tool{NewTool("bash", "run command", `{"type":"object"}`)},
	}, func(delta string) { text.WriteString(delta) }, func(delta string) { think.WriteString(delta) })
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "done" || text.String() != "done" || think.String() != "plan" {
		t.Fatalf("message = %+v, text = %q, think = %q", message, text.String(), think.String())
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "bash" || message.ToolCalls[0].Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("tool calls = %+v", message.ToolCalls)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.Cached() != 5 {
		t.Fatalf("usage = %+v", usage)
	}
	system := body["system"].([]any)
	if system[0].(map[string]any)["text"] != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("system = %#v", system)
	}
	if body["thinking"].(map[string]any)["type"] != "adaptive" || body["output_config"].(map[string]any)["effort"] != "high" {
		t.Fatalf("reasoning = %#v %#v", body["thinking"], body["output_config"])
	}
	tools := body["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "Bash" {
		t.Fatalf("tools = %#v", tools)
	}
	messages := body["messages"].([]any)
	if messages[1].(map[string]any)["role"] != "assistant" || messages[2].(map[string]any)["role"] != "user" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestClaudeComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "wrong accept", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"summary"}],"usage":{"input_tokens":9,"output_tokens":2}}`))
	}))
	defer server.Close()

	text, usage, err := NewClaude(server.URL, claudeSource(t)).Complete(context.Background(), Request{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{{Role: "system", Content: "summarize"}, {Role: "user", Content: "history"}},
	})
	if err != nil || text != "summary" || usage.PromptTokens != 9 || usage.CompletionTokens != 2 {
		t.Fatalf("complete = %q, %+v, %v", text, usage, err)
	}
}

func TestClaudeModelsReturnsFallback(t *testing.T) {
	models, err := NewClaude("https://api.anthropic.com", nil).Models(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "claude-sonnet-4-6" || !models[0].SupportsVision() {
		t.Fatalf("models = %+v, %v", models, err)
	}
}

func claudeSource(t *testing.T) *claudeauth.Source {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".whip", "claude.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"access":"access","refresh":"refresh","expires":4102444800}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return &claudeauth.Source{HomeDir: home}
}
