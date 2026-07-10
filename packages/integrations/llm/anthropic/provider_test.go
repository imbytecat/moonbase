package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llmint "github.com/imbytecat/moonbase/integrations/llm"
)

func TestCompleteUsesMessagesAPIAndSystemPrompt(t *testing.T) {
	var path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"你好"}],"model":"model","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	got, err := complete(t.Context(), providerConfig{BaseURL: srv.URL, APIKey: "key", Model: "model"}, llmint.Prompt{System: "系统", User: "用户"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "你好" || !strings.Contains(path, "messages") {
		t.Fatalf("got=%q path=%q", got, path)
	}
	if body["system"] == nil {
		t.Fatalf("body=%+v", body)
	}
}
