package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const completion = `{"id":"1","object":"chat.completion","created":1,"model":"m",
"choices":[{"index":0,"finish_reason":"stop","message":%s}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`

// modelServer imitates a model; obedient=false never calls tools.
func modelServer(t *testing.T, obedient bool) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)

		msg := `{"role":"assistant","content":"Париж"}`
		switch {
		case body["tools"] != nil && obedient:
			msg = `{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function",
"function":{"name":"search_docs","arguments":"{\"query\":\"dev-окружение\"}"}}]}`
		case body["response_format"] != nil:
			msg = `{"role":"assistant","content":"{\"answer\":\"Париж\"}"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Replace(completion, "%s", msg, 1))
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func useModel(t *testing.T, url string) {
	t.Setenv("LLM_PROVIDER", "openai_compat")
	t.Setenv("LLM_BASE_URL", url)
	t.Setenv("LLM_MODEL", "m")
}

func TestLLMCheckPassesAgainstWorkingModel(t *testing.T) {
	useModel(t, modelServer(t, true))
	var stdout, stderr bytes.Buffer

	code := run([]string{"llm-check"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	for _, probe := range []string{"text", "tool call", "json"} {
		if !strings.Contains(stdout.String(), probe+": ok") {
			t.Errorf("stdout misses %q: %q", probe+": ok", stdout.String())
		}
	}
}

func TestLLMCheckFailsWhenModelSkipsTools(t *testing.T) {
	useModel(t, modelServer(t, false))
	var stdout, stderr bytes.Buffer

	code := run([]string{"llm-check"}, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stdout.String(), "tool call: FAIL") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestLLMCheckReportsUnreachableModel(t *testing.T) {
	useModel(t, "http://127.0.0.1:1")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"llm-check"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stdout.String(), "text: FAIL") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestLLMCheckNeedsARealProvider(t *testing.T) {
	for _, provider := range []string{"anthropic", "fake"} {
		t.Setenv("LLM_PROVIDER", provider)
		var stdout, stderr bytes.Buffer
		if code := run([]string{"llm-check"}, &stdout, &stderr); code != exitError {
			t.Errorf("%s: exit %d, want %d", provider, code, exitError)
		}
	}
}

func TestLLMCheckReportsInvalidConfig(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gpt")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"llm-check"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
}
