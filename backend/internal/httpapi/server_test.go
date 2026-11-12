package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

const secret = "test-session-secret-0123456789"

var (
	fixtureOnce sync.Once
	fixture     Deps
	fixtureErr  error
)

// deps builds accounts once: bcrypt hashing per test would dominate the run time.
func deps(t *testing.T) Deps {
	t.Helper()
	fixtureOnce.Do(func() {
		data, err := seed.Load(seed.Files)
		if err != nil {
			fixtureErr = err
			return
		}
		accounts, err := auth.NewAccounts(data.Credentials())
		if err != nil {
			fixtureErr = err
			return
		}
		tokens, err := auth.NewTokens(secret, time.Hour)
		if err != nil {
			fixtureErr = err
			return
		}
		fixture = Deps{Accounts: accounts, Tokens: tokens, Directory: data.Directory()}
	})
	if fixtureErr != nil {
		t.Fatalf("fixture: %v", fixtureErr)
	}
	return fixture
}

func newServerWith(t *testing.T, d Deps) *httptest.Server {
	t.Helper()
	h, err := New(context.Background(), d)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newServerWith(t, deps(t))
}

func call(t *testing.T, s *httptest.Server, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, s.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func login(t *testing.T, s *httptest.Server, user, password string) string {
	t.Helper()
	code, body := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"`+user+`","password":"`+password+`"}`)
	if code != http.StatusOK {
		t.Fatalf("login %s: %d %v", user, code, body)
	}
	return body["token"].(string)
}

func TestLoginReturnsTokenAndSnakeCaseUser(t *testing.T) {
	s := newServer(t)

	code, body := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`)

	if code != http.StatusOK || body["token"] == "" {
		t.Fatalf("status %d, body %v", code, body)
	}
	user := body["user"].(map[string]any)
	if user["display_name"] != "Лиза Орлова" || user["role"] != "ROLE_INTERN" || user["team_id"] != "t-messenger" {
		t.Errorf("user = %v", user)
	}
	if _, camel := user["displayName"]; camel {
		t.Error("camelCase field in API JSON")
	}
}

func TestLoginRejectsBadCredentialsTheSameWay(t *testing.T) {
	s := newServer(t)

	codeA, wrongPassword := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"nope"}`)
	codeB, unknownLogin := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"ghost","password":"nope"}`)

	if codeA != http.StatusUnauthorized || codeB != http.StatusUnauthorized {
		t.Fatalf("statuses %d, %d, want 401", codeA, codeB)
	}
	if wrongPassword["message"] != unknownLogin["message"] {
		t.Errorf("messages differ: %v vs %v", wrongPassword["message"], unknownLogin["message"])
	}
}

func TestLoginRejectsMalformedBody(t *testing.T) {
	s := newServer(t)
	if code, _ := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":`); code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", code)
	}
}

func TestPrivatePathsNeedAValidSession(t *testing.T) {
	s := newServer(t)
	expiredIssuer, _ := auth.NewTokens(secret, -time.Minute)
	ghostIssuer, _ := auth.NewTokens(secret, time.Hour)

	for name, token := range map[string]string{
		"no token":       "",
		"garbage":        "garbage",
		"expired":        expiredIssuer.Issue("u-liza"),
		"unknown user":   ghostIssuer.Issue("u-ghost"),
		"foreign secret": mustTokens(t, "another-secret-0123456789").Issue("u-liza"),
	} {
		t.Run(name, func(t *testing.T) {
			code, body := call(t, s, http.MethodGet, "/v1/private/me", token, "")
			if code != http.StatusUnauthorized || body["code"] != float64(16) {
				t.Fatalf("status %d, body %v; want 401 with code 16", code, body)
			}
		})
	}
}

func TestPrivateRejectsNonBearerScheme(t *testing.T) {
	s := newServer(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL+"/v1/private/me", nil)
	req.Header.Set("Authorization", "Basic aW50ZXJuOmludGVybi1kZW1v")
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func mustTokens(t *testing.T, s string) *auth.Tokens {
	t.Helper()
	tok, err := auth.NewTokens(s, time.Hour)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	return tok
}

func TestMeReturnsProfileTeamAndRelations(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")

	code, body := call(t, s, http.MethodGet, "/v1/private/me", token, "")

	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, body)
	}
	user := body["user"].(map[string]any)
	team := body["team"].(map[string]any)
	rel := body["relations"].(map[string]any)
	if user["login"] != "intern" || user["start_date"] != "2026-11-09" || user["direction"] != "DIRECTION_BACKEND" {
		t.Errorf("user = %v", user)
	}
	if team["name"] != "Messenger" || team["cluster"] != "Коммуникации" {
		t.Errorf("team = %v", team)
	}
	for field, name := range map[string]string{"mentor": "Анна Белова", "manager": "Дмитрий Козлов", "hr_bp": "Елена Соколова"} {
		ref, _ := rel[field].(map[string]any)
		if ref == nil || ref["name"] != name {
			t.Errorf("relations.%s = %v, want %s", field, rel[field], name)
		}
	}
}

func TestMeOfManagerHasNoMentor(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "manager", "manager-demo")

	_, body := call(t, s, http.MethodGet, "/v1/private/me", token, "")

	rel := body["relations"].(map[string]any)
	if rel["mentor"] != nil {
		t.Errorf("manager mentor = %v, want null", rel["mentor"])
	}
	if ref, _ := rel["hr_bp"].(map[string]any); ref == nil || ref["name"] != "Елена Соколова" {
		t.Errorf("manager hr_bp = %v", rel["hr_bp"])
	}
}

func TestPathsOutsideTheSchemeAreNotFound(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")
	for _, path := range []string{"/v1/me", "/v1/public/me", "/v2/private/me", "/v1/private/nothing", "/"} {
		if code, _ := call(t, s, http.MethodGet, path, token, ""); code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", path, code)
		}
	}
}

func TestHealthz(t *testing.T) {
	s := newServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		if code, _ := call(t, s, http.MethodGet, path, "", ""); code != http.StatusOK {
			t.Errorf("GET %s: status %d", path, code)
		}
	}
}

// Every path in the generated contract must sit under /v1/public/ or /v1/private/.
func TestContractPathsFollowTheScheme(t *testing.T) {
	f, err := os.Open("../../api/openapi/buddy-api.yaml")
	if err != nil {
		t.Fatalf("open contract: %v", err)
	}
	defer f.Close()

	found := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "    /") {
			continue
		}
		found++
		path := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if !strings.HasPrefix(path, "/v1/public/") && !strings.HasPrefix(path, "/v1/private/") {
			t.Errorf("contract path %s is neither /v1/public/ nor /v1/private/", path)
		}
	}
	if found == 0 {
		t.Fatal("no paths found in the contract")
	}
}
