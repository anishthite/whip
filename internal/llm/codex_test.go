package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/context-labs/whip/internal/codexauth"
)

func TestCodexStreamRequestAndEvents(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/codex/responses" {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		for header, expected := range map[string]string{
			"Authorization":      "Bearer access",
			"ChatGPT-Account-ID": "account",
			"Originator":         "whip",
			"OpenAI-Beta":        "responses=experimental",
			"Accept":             "text/event-stream",
			"Content-Type":       "application/json",
		} {
			if value := r.Header.Get(header); value != expected {
				http.Error(w, header+" = "+value, http.StatusBadRequest)
				return
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"plan\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"message\",\"id\":\"msg-1\",\"phase\":\"commentary\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"fc-1\",\"call_id\":\"call-1\",\"name\":\"bash\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call-1\",\"delta\":\"{\\\"command\\\":\\\"p\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.done\",\"call_id\":\"call-1\",\"arguments\":\"{\\\"command\\\":\\\"pwd\\\"}\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":7,\"input_tokens_details\":{\"cached_tokens\":5}}}}\n\n")
	}))
	defer srv.Close()

	tool := NewTool("bash", "run a command", `{"type":"object"}`)
	var text, think strings.Builder
	msg, usage, err := NewCodex(srv.URL, codexSource(t)).Stream(context.Background(), Request{
		Model:           "gpt-5.4",
		MaxTokens:       128000,
		ReasoningEffort: "high",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "inspect the repository"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "old-call", ItemID: "fc-old", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path":"README.md"}`}}}},
			{Role: "tool", ToolCallID: "old-call", Name: "read", Content: "file contents"},
		},
		Tools: []Tool{tool},
	}, func(delta string) { text.WriteString(delta) }, func(delta string) { think.WriteString(delta) })
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "done" || text.String() != "done" || think.String() != "plan" {
		t.Fatalf("message streams: msg=%+v text=%q think=%q", msg, text.String(), think.String())
	}
	if msg.ResponseID != "msg-1" {
		t.Fatalf("response message ID = %q", msg.ResponseID)
	}
	if msg.ResponsePhase != "commentary" {
		t.Fatalf("response message phase = %q", msg.ResponsePhase)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call-1" || msg.ToolCalls[0].Function.Name != "bash" || msg.ToolCalls[0].Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("tool calls: %+v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].ItemID != "fc-1" {
		t.Fatalf("tool call item ID = %q", msg.ToolCalls[0].ItemID)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.Cached() != 5 {
		t.Fatalf("usage: %+v", usage)
	}
	if got["model"] != "gpt-5.4" || got["instructions"] != "system prompt" || got["stream"] != true || got["store"] != false || got["tool_choice"] != "auto" || got["parallel_tool_calls"] != true {
		t.Fatalf("request = %#v", got)
	}
	if _, ok := got["max_output_tokens"]; ok {
		t.Fatalf("Codex subscription request must omit max_output_tokens: %#v", got)
	}
	reasoning, ok := got["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("reasoning = %#v", got["reasoning"])
	}
	input, ok := got["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("input = %#v", got["input"])
	}
	if input[1].(map[string]any)["type"] != "function_call" || input[2].(map[string]any)["type"] != "function_call_output" {
		t.Fatalf("tool history input = %#v", input)
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 || tools[0].(map[string]any)["name"] != "bash" {
		t.Fatalf("tools = %#v", got["tools"])
	}
}

func TestCodexComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"stream":true`) {
			http.Error(w, "complete streamed", http.StatusBadRequest)
			return
		}
		if strings.Contains(string(body), `"max_output_tokens"`) {
			http.Error(w, "max_output_tokens is not accepted by Codex subscriptions", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":"summary"}]}],"usage":{"input_tokens":9,"output_tokens":2}}`)
	}))
	defer srv.Close()

	text, usage, err := NewCodex(srv.URL, codexSource(t)).Complete(context.Background(), Request{
		Model:    "gpt-5.4",
		Messages: []Message{{Role: "system", Content: "summarize"}, {Role: "user", Content: "history"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "summary" || usage.PromptTokens != 9 || usage.CompletionTokens != 2 {
		t.Fatalf("complete = %q, %+v", text, usage)
	}
}

// Codex's Responses endpoint distinguishes a previous assistant output from a
// new input message. In particular, output_text belongs to a completed message
// item with an ID; sending it as a bare assistant message is rejected by the
// subscription backend as an unknown content parameter.
func TestCodexRequestUsesOutputMessageForAssistantHistory(t *testing.T) {
	call := ToolCall{ID: "call-1", ItemID: "fc-1", Type: "function"}
	call.Function.Name = "read"
	call.Function.Arguments = `{"path":"README.md"}`
	body := codexRequest(Request{
		Model: "gpt-5.6-terra",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "inspect the repository"},
			{Role: "assistant", Content: "I will inspect it.", ResponseID: "msg-1", ResponsePhase: "commentary", ToolCalls: []ToolCall{call}},
			{Role: "tool", ToolCallID: "call-1", Content: "README contents"},
		},
	}, true)

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	input := got["input"].([]any)
	if len(input) != 4 {
		t.Fatalf("input = %#v", input)
	}
	user := input[0].(map[string]any)
	if user["role"] != "user" {
		t.Fatalf("user input = %#v", user)
	}
	if _, ok := user["type"]; ok {
		t.Fatalf("user input must not carry a type discriminator: %#v", user)
	}
	assistant := input[1].(map[string]any)
	if assistant["type"] != "message" || assistant["role"] != "assistant" {
		t.Fatalf("assistant history = %#v", assistant)
	}
	if assistant["id"] != "msg-1" || assistant["status"] != "completed" {
		t.Fatalf("assistant output identity = %#v", assistant)
	}
	if assistant["phase"] != "commentary" {
		t.Fatalf("assistant output phase = %#v", assistant)
	}
	content, ok := assistant["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("assistant content = %#v", assistant["content"])
	}
	text := content[0].(map[string]any)
	if text["type"] != "output_text" || text["text"] != "I will inspect it." {
		t.Fatalf("assistant text = %#v", text)
	}
	if annotations, ok := text["annotations"].([]any); !ok || len(annotations) != 0 {
		t.Fatalf("assistant annotations = %#v", text["annotations"])
	}
	callItem := input[2].(map[string]any)
	if callItem["type"] != "function_call" || callItem["id"] != "fc-1" || callItem["call_id"] != "call-1" {
		t.Fatalf("assistant tool call = %#v", callItem)
	}
}

func TestCodexMessageID(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  Message
		want string
	}{
		{name: "provider ID", msg: Message{ResponseID: "msg-provider"}, want: "msg-provider"},
		{name: "older session", want: "msg_3"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexMessageID(tt.msg, 3); got != tt.want {
				t.Fatalf("codexMessageID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodexRequestFlattensLegacyToolHistory(t *testing.T) {
	legacy := ToolCall{ID: "call-old", Type: "function"}
	legacy.Function.Name = "read"
	legacy.Function.Arguments = `{"path":"README.md"}`
	body := codexRequest(Request{
		Model: "gpt-5.6-terra",
		Messages: []Message{
			{Role: "user", Content: "inspect the repository"},
			{Role: "assistant", ToolCalls: []ToolCall{legacy}},
			{Role: "tool", ToolCallID: "call-old", Content: "README contents"},
			{Role: "user", Content: "continue"},
		},
	}, true)

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	input := got["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input = %#v", input)
	}
	for _, item := range input {
		message := item.(map[string]any)
		if _, ok := message["type"]; ok {
			t.Fatalf("legacy history must not emit native Responses items: %#v", message)
		}
	}
	legacyContext := input[1].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if legacyContext != "[Earlier tool activity]\n\n[Tool call]\nread({\"path\":\"README.md\"})\n\n[Tool result]\nREADME contents" {
		t.Fatalf("legacy context = %q", legacyContext)
	}
}

func TestCodexModelsFetchesAccountCatalog(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/codex/models" {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("client_version"); got != codexModelsClientVersion {
			http.Error(w, "missing client_version", http.StatusBadRequest)
			return
		}
		gotHeaders = r.Header.Clone()
		fmt.Fprint(w, `{"models":[
  {"slug":"gpt-5.6-sol","supported_in_api":true,"context_window":1050000,"supported_reasoning_levels":[{"effort":"none"},{"effort":"low"},{"effort":"max"}],"input_modalities":["text","image"]},
  {"slug":"gpt-rollout","supported_in_api":false,"context_window":1000},
  {"slug":"gpt-5.4","supported_in_api":true,"max_context_window":272000,"supported_reasoning_levels":[{"effort":"medium"}]}
]}`)
	}))
	defer srv.Close()

	models, err := NewCodex(srv.URL, codexSource(t)).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotHeaders.Get("Authorization") != "Bearer access" || gotHeaders.Get("ChatGPT-Account-ID") != "account" || gotHeaders.Get("Originator") != "whip" {
		t.Fatalf("catalog auth headers = %#v", gotHeaders)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v, want two supported entries", models)
	}
	if got := models[0]; got.ID != "gpt-5.6-sol" || got.ContextLength != 1050000 || !got.SupportsVision() || strings.Join(got.ReasoningEfforts, ",") != "none,low,max" {
		t.Fatalf("first model = %+v", got)
	}
	if got := models[1]; got.ID != "gpt-5.4" || got.ContextLength != 272000 || !got.SupportsVision() || strings.Join(got.ReasoningEfforts, ",") != "medium" {
		t.Fatalf("second model = %+v", got)
	}
}

func TestCodexModelsReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not entitled", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := NewCodex(srv.URL, codexSource(t)).Models(context.Background())
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != "403 Forbidden" || !strings.Contains(httpErr.Body, "not entitled") {
		t.Fatalf("error = %#v, want typed 403", err)
	}
}

func codexSource(t *testing.T) *codexauth.Source {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh","account_id":"account"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return &codexauth.Source{HomeDir: home}
}
