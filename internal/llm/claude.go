package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/context-labs/whip/internal/claudeauth"
)

const claudeCodeVersion = "2.1.75"

// Claude maps Whip's provider-neutral agent loop onto Anthropic Messages with
// the OAuth request conventions used by Pi Coding Agent.
type Claude struct {
	BaseURL string
	Source  *claudeauth.Source
	HTTP    *http.Client
}

var _ Client = (*Claude)(nil)

func NewClaude(baseURL string, source *claudeauth.Source) *Claude {
	return &Claude{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Source:  source,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// Models returns the fixed fallback route. Claude subscription OAuth exposes
// no account-scoped model catalog, so availability remains user-configurable.
func (c *Claude) Models(context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{
		ID:                  "claude-sonnet-4-6",
		ContextLength:       200000,
		MaxCompletionTokens: 64000,
		ReasoningEfforts:    []string{"low", "medium", "high"},
		InputModalities:     []string{"text", "image"},
	}}, nil
}

func (c *Claude) Stream(ctx context.Context, req Request, onText, onThink func(string)) (Message, Usage, error) {
	body, err := claudeRequest(req, true)
	if err != nil {
		return Message{}, Usage{}, err
	}
	resp, err := c.post(ctx, body, true)
	if err != nil {
		return Message{}, Usage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Message{}, Usage{}, httpError(resp)
	}

	msg := Message{Role: "assistant"}
	var usage Usage
	partial := map[int]string{}
	calls := map[int]*ToolCall{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event claudeEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			continue
		}
		switch event.Type {
		case "error":
			if event.Error.Message != "" {
				return Message{}, usage, fmt.Errorf("Claude API error: %s", event.Error.Message)
			}
			return Message{}, usage, errors.New("Claude response failed")
		case "message_start":
			usage = event.Message.Usage.usage()
		case "content_block_start":
			switch event.ContentBlock.Type {
			case "text":
				msg.Content += event.ContentBlock.Text
				if onText != nil && event.ContentBlock.Text != "" {
					onText(event.ContentBlock.Text)
				}
			case "thinking":
				if onThink != nil && event.ContentBlock.Thinking != "" {
					onThink(event.ContentBlock.Thinking)
				}
			case "tool_use":
				call := ToolCall{ID: event.ContentBlock.ID, Type: "function"}
				call.Function.Name = claudeToolName(event.ContentBlock.Name, req.Tools)
				call.Function.Arguments = compactJSON(event.ContentBlock.Input)
				calls[event.Index] = &call
				msg.ToolCalls = append(msg.ToolCalls, call)
			}
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				msg.Content += event.Delta.Text
				if onText != nil && event.Delta.Text != "" {
					onText(event.Delta.Text)
				}
			case "thinking_delta":
				if onThink != nil && event.Delta.Thinking != "" {
					onThink(event.Delta.Thinking)
				}
			case "input_json_delta":
				partial[event.Index] += event.Delta.PartialJSON
				if call := calls[event.Index]; call != nil {
					call.Function.Arguments = partial[event.Index]
					for i := range msg.ToolCalls {
						if msg.ToolCalls[i].ID == call.ID {
							msg.ToolCalls[i].Function.Arguments = call.Function.Arguments
						}
					}
				}
			}
		case "message_delta":
			usage = event.Usage.usage()
		}
	}
	if err := scanner.Err(); err != nil {
		return Message{}, usage, err
	}
	for i := range msg.ToolCalls {
		if json.Valid([]byte(msg.ToolCalls[i].Function.Arguments)) {
			continue
		}
		msg.ToolCalls[i].Function.Arguments = "{}"
	}
	return msg, usage, nil
}

func (c *Claude) Complete(ctx context.Context, req Request) (string, Usage, error) {
	body, err := claudeRequest(req, false)
	if err != nil {
		return "", Usage{}, err
	}
	resp, err := c.post(ctx, body, false)
	if err != nil {
		return "", Usage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", Usage{}, httpError(resp)
	}
	var response claudeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&response); err != nil {
		return "", Usage{}, err
	}
	var text strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return "", response.Usage.usage(), errors.New("no text in Claude completion response")
	}
	return text.String(), response.Usage.usage(), nil
}

func (c *Claude) post(ctx context.Context, body claudeRequestBody, stream bool) (*http.Response, error) {
	if c.Source == nil {
		return nil, claudeauth.ErrLoginRequired
	}
	creds, err := c.Source.Credentials(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
	req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Header.Set("user-agent", "claude-cli/"+claudeCodeVersion)
	req.Header.Set("x-app", "cli")
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	return c.httpClient().Do(req)
}

func (c *Claude) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

type claudeRequestBody struct {
	Model        string            `json:"model"`
	System       []claudeContent   `json:"system,omitempty"`
	Messages     []claudeMessage   `json:"messages"`
	MaxTokens    int               `json:"max_tokens"`
	Tools        []claudeTool      `json:"tools,omitempty"`
	Thinking     map[string]string `json:"thinking,omitempty"`
	OutputConfig map[string]string `json:"output_config,omitempty"`
	Stream       bool              `json:"stream"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	Source    *claudeImage    `json:"source,omitempty"`
}

type claudeImage struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func claudeRequest(req Request, stream bool) (claudeRequestBody, error) {
	req.Messages = repairToolHistory(stripAuthored(req.Messages))
	body := claudeRequestBody{Model: req.Model, MaxTokens: req.MaxTokens, Stream: stream}
	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}
	body.System = append(body.System, claudeContent{Type: "text", Text: "You are Claude Code, Anthropic's official CLI for Claude."})
	for _, msg := range req.Messages {
		if msg.Role == "system" && msg.TextContent() != "" {
			body.System = append(body.System, claudeContent{Type: "text", Text: msg.TextContent()})
		}
	}
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			continue
		}
		blocks, err := claudeBlocks(msg)
		if err != nil {
			return claudeRequestBody{}, err
		}
		if len(blocks) == 0 {
			continue
		}
		role := msg.Role
		if role == "tool" {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		if n := len(body.Messages); n > 0 && body.Messages[n-1].Role == role {
			body.Messages[n-1].Content = append(body.Messages[n-1].Content, blocks...)
		} else {
			body.Messages = append(body.Messages, claudeMessage{Role: role, Content: blocks})
		}
	}
	for _, tool := range req.Tools {
		body.Tools = append(body.Tools, claudeTool{
			Name:        toClaudeCodeName(tool.Function.Name),
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	if req.ReasoningEffort != "" {
		body.Thinking = map[string]string{"type": "adaptive"}
		body.OutputConfig = map[string]string{"effort": req.ReasoningEffort}
	}
	return body, nil
}

func claudeBlocks(msg Message) ([]claudeContent, error) {
	switch msg.Role {
	case "tool":
		return []claudeContent{{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: msg.Content}}, nil
	case "assistant":
		blocks := []claudeContent{}
		if text := msg.TextContent(); text != "" {
			blocks = append(blocks, claudeContent{Type: "text", Text: text})
		}
		for _, call := range msg.ToolCalls {
			input := json.RawMessage(call.Function.Arguments)
			if !json.Valid(input) {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, claudeContent{Type: "tool_use", ID: call.ID, Name: toClaudeCodeName(call.Function.Name), Input: input})
		}
		return blocks, nil
	case "user":
		blocks := []claudeContent{}
		if text := msg.TextContent(); text != "" {
			blocks = append(blocks, claudeContent{Type: "text", Text: text})
		}
		for _, part := range msg.Parts {
			image, ok := claudeImagePart(part)
			if !ok {
				continue
			}
			blocks = append(blocks, claudeContent{Type: "image", Source: image})
		}
		return blocks, nil
	}
	return nil, nil
}

func claudeImagePart(part ContentPart) (*claudeImage, bool) {
	if part.Type != "image_url" || part.ImageURL == nil {
		return nil, false
	}
	pieces := strings.SplitN(strings.TrimPrefix(part.ImageURL.URL, "data:"), ",", 2)
	if len(pieces) != 2 || !strings.HasSuffix(pieces[0], ";base64") {
		return nil, false
	}
	if _, err := base64.StdEncoding.DecodeString(pieces[1]); err != nil {
		return nil, false
	}
	return &claudeImage{Type: "base64", MediaType: strings.TrimSuffix(pieces[0], ";base64"), Data: pieces[1]}, true
}

func compactJSON(raw json.RawMessage) string {
	if !json.Valid(raw) {
		return "{}"
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "{}"
	}
	data, _ := json.Marshal(value)
	return string(data)
}

var claudeCodeToolNames = map[string]string{
	"read": "Read", "write": "Write", "edit": "Edit", "bash": "Bash", "grep": "Grep", "glob": "Glob", "task": "Task", "todowrite": "TodoWrite", "webfetch": "WebFetch", "websearch": "WebSearch",
}

func toClaudeCodeName(name string) string {
	if canonical, ok := claudeCodeToolNames[strings.ToLower(name)]; ok {
		return canonical
	}
	return name
}

func claudeToolName(name string, tools []Tool) string {
	for _, tool := range tools {
		if strings.EqualFold(toClaudeCodeName(tool.Function.Name), name) {
			return tool.Function.Name
		}
	}
	return name
}

type claudeEvent struct {
	Type         string        `json:"type"`
	Index        int           `json:"index"`
	ContentBlock claudeContent `json:"content_block"`
	Delta        struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Message claudeResponse `json:"message"`
	Usage   claudeUsage    `json:"usage"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
}

type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Usage   claudeUsage     `json:"usage"`
}

type claudeUsage struct {
	InputTokens             int `json:"input_tokens"`
	OutputTokens            int `json:"output_tokens"`
	CacheReadInputTokens    int `json:"cache_read_input_tokens"`
	CacheCreationInputToken int `json:"cache_creation_input_tokens"`
}

func (u claudeUsage) usage() Usage {
	out := Usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens}
	if u.CacheReadInputTokens > 0 {
		out.PromptTokensDetails = &struct {
			CachedTokens int `json:"cached_tokens"`
		}{CachedTokens: u.CacheReadInputTokens}
	}
	return out
}
