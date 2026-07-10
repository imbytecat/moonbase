package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llmint "github.com/imbytecat/moonbase/integrations/llm"
)

func TestCompleteUsesCustomBaseURLAndSystemPrompt(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	}))
	defer srv.Close()
	got, err := complete(
		t.Context(),
		providerConfig{BaseURL: srv.URL, APIKey: "key", Model: "model"},
		llmint.Prompt{System: "系统", User: "用户"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "你好" {
		t.Fatalf("got=%q", got)
	}
	raw, _ := json.Marshal(body["messages"])
	if !strings.Contains(string(raw), "系统") || !strings.Contains(string(raw), "用户") {
		t.Fatalf("messages=%s", raw)
	}
}
func TestCompleteRejectsEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	_, err := complete(
		t.Context(),
		providerConfig{BaseURL: srv.URL, APIKey: "key", Model: "model"},
		llmint.Prompt{User: "hi"},
	)
	if err == nil {
		t.Fatal("empty response must fail")
	}
}
