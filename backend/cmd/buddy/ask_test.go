package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskStreamsTheAnswer(t *testing.T) {
	useModel(t, modelServer(t, true))
	var stdout, stderr bytes.Buffer

	code := run([]string{"ask", "-as", "intern", "-v", "Столица Франции?"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Париж") || !strings.Contains(stdout.String(), "message_sent") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestAskRejectsBadInput(t *testing.T) {
	useModel(t, modelServer(t, true))
	for name, args := range map[string][]string{
		"no question":    {"ask"},
		"unknown login":  {"ask", "-as", "ghost", "привет"},
		"unknown thread": {"ask", "-thread", "chat", "привет"},
		"bad flag":       {"ask", "-loud", "привет"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitError {
				t.Fatalf("exit %d, want %d", code, exitError)
			}
		})
	}
}

func TestAskReportsModelFailure(t *testing.T) {
	useModel(t, "http://127.0.0.1:1")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ask", "привет"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
}
