package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectEvents(ch <-chan Event) []Event {
	out := make([]Event, 0, 4)
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestOpenAIAdapterStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello from openai"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventTextDelta || evs[1].Delta != "hello from openai" {
		t.Fatalf("unexpected text event: %+v", evs[1])
	}
}

func TestOpenAIAdapterToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call-1",
								"type": "function",
								"function": map[string]any{
									"name":      "lookup",
									"arguments": `{"q":"go"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventToolCall {
		t.Fatalf("expected tool call event, got %+v", evs[1])
	}
	if evs[1].ToolCall.Name != "lookup" {
		t.Fatalf("unexpected tool call name: %+v", evs[1].ToolCall)
	}
	if got, _ := evs[1].ToolCall.Arguments["q"].(string); got != "go" {
		t.Fatalf("unexpected tool call args: %+v", evs[1].ToolCall.Arguments)
	}
	if evs[2].Type != EventAwaitNext {
		t.Fatalf("expected await-next event, got %+v", evs[2])
	}
}

func TestOpenAIAdapterTextToolCallStaysAsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```json\n{\n  \"name\": \"read registered runtime tool\",\n  \"arguments\": {\"path\": \"/tmp/a.txt\"}\n}\n```",
					},
				},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "read file"}},
		ActiveTools: []string{"read", "ls"},
	}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventTextDelta {
		t.Fatalf("expected text event, got %+v", evs[1])
	}
	if !strings.Contains(evs[1].Delta, "\"name\": \"read registered runtime tool\"") {
		t.Fatalf("unexpected text payload: %+v", evs[1])
	}
}

func TestGeminiAdapterStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "hello "}, {"text": "gemini"}}}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewGeminiAdapter("test-key", "gemini-test", srv.URL)
	if err != nil {
		t.Fatalf("new gemini adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventTextDelta || evs[1].Delta != "hello gemini" {
		t.Fatalf("unexpected text event: %+v", evs[1])
	}
}

func TestOpenAIAdapterSendsActiveTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawTools, ok := req["tools"].([]any)
		if !ok || len(rawTools) != 2 {
			t.Fatalf("expected two tools in request, got: %#v", req["tools"])
		}
		readTool, ok := rawTools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool payload must be object: %#v", rawTools[0])
		}
		fn, ok := readTool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function payload missing: %#v", readTool)
		}
		params, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing: %#v", fn)
		}
		required, ok := params["required"].([]any)
		if !ok || len(required) == 0 || required[0] != "path" {
			t.Fatalf("expected required path for read schema, got: %#v", params["required"])
		}
		grepTool, ok := rawTools[1].(map[string]any)
		if !ok {
			t.Fatalf("tool payload must be object: %#v", rawTools[1])
		}
		grepFn, ok := grepTool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function payload missing: %#v", grepTool)
		}
		grepParams, ok := grepFn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("grep parameters missing: %#v", grepFn)
		}
		grepRequired, ok := grepParams["required"].([]any)
		if !ok || len(grepRequired) == 0 || grepRequired[0] != "pattern" {
			t.Fatalf("expected required pattern for grep schema, got: %#v", grepParams["required"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		ActiveTools: []string{"read", "grep"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterSendsToolResultAsFollowupContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawMsgs, ok := req["messages"].([]any)
		if !ok || len(rawMsgs) < 2 {
			t.Fatalf("expected two messages, got: %#v", req["messages"])
		}
		last, _ := rawMsgs[len(rawMsgs)-1].(map[string]any)
		content, _ := last["content"].(string)
		if !strings.Contains(content, "Tool result:") || !strings.Contains(content, "first => ok") {
			t.Fatalf("tool result message missing, got: %q", content)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "tool_result", Content: "first => ok"},
		},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterUsesStructuredMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawMsgs, ok := req["messages"].([]any)
		if !ok || len(rawMsgs) != 2 {
			t.Fatalf("expected two structured messages, got: %#v", req["messages"])
		}
		first, _ := rawMsgs[0].(map[string]any)
		if role, _ := first["role"].(string); role != "user" {
			t.Fatalf("unexpected first role: %#v", first)
		}
		if content, _ := first["content"].(string); content != "please summarize" {
			t.Fatalf("unexpected first content: %#v", first)
		}
		second, _ := rawMsgs[1].(map[string]any)
		if role, _ := second["role"].(string); role != "user" {
			t.Fatalf("tool_result should map to user role, got: %#v", second)
		}
		if content, _ := second["content"].(string); !strings.Contains(content, "Tool result:") {
			t.Fatalf("expected tool_result wrapper content, got: %#v", second)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: "user", Content: "please summarize"},
			{Role: "tool_result", Content: "first => ok"},
		},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterWriteToolSchemaRequiresPathAndContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawTools, ok := req["tools"].([]any)
		if !ok || len(rawTools) != 1 {
			t.Fatalf("expected one tool in request, got: %#v", req["tools"])
		}
		writeTool, ok := rawTools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool payload must be object: %#v", rawTools[0])
		}
		fn, ok := writeTool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function payload missing: %#v", writeTool)
		}
		params, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing: %#v", fn)
		}
		required, ok := params["required"].([]any)
		if !ok || len(required) != 2 || required[0] != "path" || required[1] != "content" {
			t.Fatalf("expected required path+content for write schema, got: %#v", params["required"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		ActiveTools: []string{"write"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterEditToolSchemaRequiresPathOldNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawTools, ok := req["tools"].([]any)
		if !ok || len(rawTools) != 1 {
			t.Fatalf("expected one tool in request, got: %#v", req["tools"])
		}
		editTool, ok := rawTools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool payload must be object: %#v", rawTools[0])
		}
		fn, ok := editTool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function payload missing: %#v", editTool)
		}
		params, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing: %#v", fn)
		}
		required, ok := params["required"].([]any)
		if !ok || len(required) != 3 || required[0] != "path" || required[1] != "oldText" || required[2] != "newText" {
			t.Fatalf("expected required path+oldText+newText for edit schema, got: %#v", params["required"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		ActiveTools: []string{"edit"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterBashToolSchemaRequiresCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawTools, ok := req["tools"].([]any)
		if !ok || len(rawTools) != 1 {
			t.Fatalf("expected one tool in request, got: %#v", req["tools"])
		}
		bashTool, ok := rawTools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool payload must be object: %#v", rawTools[0])
		}
		fn, ok := bashTool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function payload missing: %#v", bashTool)
		}
		params, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing: %#v", fn)
		}
		required, ok := params["required"].([]any)
		if !ok || len(required) != 1 || required[0] != "command" {
			t.Fatalf("expected required command for bash schema, got: %#v", params["required"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		ActiveTools: []string{"bash"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}
