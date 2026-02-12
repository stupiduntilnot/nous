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

func TestOpenAIAdapterRetriesRetryableHTTPThenSucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"try later"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "recovered"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}))
	if attempts != 2 {
		t.Fatalf("expected one retry attempt, got attempts=%d", attempts)
	}
	foundRetryWarning := false
	foundText := false
	for _, ev := range evs {
		if ev.Type == EventWarning && ev.Code == "provider_retry" {
			foundRetryWarning = true
		}
		if ev.Type == EventTextDelta && ev.Delta == "recovered" {
			foundText = true
		}
	}
	if !foundRetryWarning || !foundText {
		t.Fatalf("expected retry warning and recovered text, got events=%+v", evs)
	}
}

func TestOpenAIAdapterRetryExhaustedReturnsTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"error":"timeout"}`))
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}))
	last := evs[len(evs)-1]
	if last.Type != EventError {
		t.Fatalf("expected final error event, got %+v", last)
	}
	if !IsRetryExhaustedError(last.Err) {
		t.Fatalf("expected retry-exhausted error type, got %+v", last.Err)
	}
}

func TestOpenAIAdapterAbortReturnsTypedError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", "http://127.0.0.1:9")
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(ctx, Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}))
	last := evs[len(evs)-1]
	if last.Type != EventError {
		t.Fatalf("expected final error event, got %+v", last)
	}
	if !IsAbortedError(last.Err) {
		t.Fatalf("expected aborted error type, got %+v", last.Err)
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
	done := evs[len(evs)-1]
	if done.Type != EventDone || done.StopReason != StopReasonToolUse {
		t.Fatalf("expected done tool_use event, got %+v", done)
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
	done := evs[len(evs)-1]
	if done.Type != EventDone || done.StopReason == "" {
		t.Fatalf("expected done event with stop reason, got %+v", done)
	}
}

func TestOpenAIAdapterDoneIncludesUsageAndStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "length",
					"message":       map[string]any{"content": "truncated"},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}))
	done := evs[len(evs)-1]
	if done.Type != EventDone {
		t.Fatalf("expected done event, got %+v", done)
	}
	if done.StopReason != StopReasonLength {
		t.Fatalf("expected length stop reason, got %+v", done)
	}
	if done.Usage == nil || done.Usage.InputTokens != 10 || done.Usage.OutputTokens != 5 || done.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage on done event: %+v", done.Usage)
	}
}

func TestGeminiAdapterDoneIncludesUsageAndStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"finishReason": "STOP",
					"content":      map[string]any{"parts": []map[string]any{{"text": "hello gemini"}}},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     9,
				"candidatesTokenCount": 4,
				"totalTokenCount":      13,
			},
		})
	}))
	defer srv.Close()

	a, err := NewGeminiAdapter("test-key", "gemini-test", srv.URL)
	if err != nil {
		t.Fatalf("new gemini adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}))
	done := evs[len(evs)-1]
	if done.Type != EventDone {
		t.Fatalf("expected done event, got %+v", done)
	}
	if done.StopReason != StopReasonStop {
		t.Fatalf("expected stop stop reason, got %+v", done)
	}
	if done.Usage == nil || done.Usage.InputTokens != 9 || done.Usage.OutputTokens != 4 || done.Usage.TotalTokens != 13 {
		t.Fatalf("unexpected usage on done event: %+v", done.Usage)
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

func TestOpenAIAdapterMapsToolResultWithToolCallIDToToolRole(t *testing.T) {
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
		if !ok || len(rawMsgs) != 3 {
			t.Fatalf("expected three structured messages, got: %#v", req["messages"])
		}
		assistant, _ := rawMsgs[1].(map[string]any)
		if role, _ := assistant["role"].(string); role != "assistant" {
			t.Fatalf("unexpected second role: %#v", assistant)
		}
		if _, ok := assistant["tool_calls"].([]any); !ok {
			t.Fatalf("assistant tool_calls missing: %#v", assistant)
		}
		toolMsg, _ := rawMsgs[2].(map[string]any)
		if role, _ := toolMsg["role"].(string); role != "tool" {
			t.Fatalf("expected tool_result to map to tool role with call id, got: %#v", toolMsg)
		}
		if callID, _ := toolMsg["tool_call_id"].(string); callID != "call-1" {
			t.Fatalf("unexpected tool_call_id: %#v", toolMsg)
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
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "/tmp/a"}}}},
			{Role: "tool_result", Content: "read => 1", ToolCallID: "call-1"},
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
