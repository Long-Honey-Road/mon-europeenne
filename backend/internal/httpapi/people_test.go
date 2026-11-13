package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func peopleIn(body map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, p := range body["people"].([]any) {
		m := p.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

func TestListPeopleOwnTeamFirstAndContactsByCluster(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")

	code, body := call(t, s, http.MethodGet, "/v1/private/people", token, "")
	if code != http.StatusOK {
		t.Fatalf("code %d %v", code, body)
	}
	list := body["people"].([]any)
	if len(list) < 5 || list[0].(map[string]any)["team_id"] != "t-messenger" {
		t.Fatalf("people = %v", list)
	}
	byID := peopleIn(body)
	anna, pavel, artem := byID["p-anna"], byID["p-pavel"], byID["p-artem"]
	if anna["contact"] == "" || anna["other_team"] != false {
		t.Fatalf("own team keeps contact: %v", anna)
	}
	// Pavel is in another team of the same cluster, Artem in another cluster.
	if pavel["contact"] == "" || pavel["other_team"] != true {
		t.Fatalf("same cluster keeps contact: %v", pavel)
	}
	if artem["contact"] != "" || artem["other_team"] != true {
		t.Fatalf("other cluster hides contact: %v", artem)
	}
}

func TestListPeopleFilters(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")

	_, body := call(t, s, http.MethodGet, "/v1/private/people?team_id=t-platform", token, "")
	for id, p := range peopleIn(body) {
		if p["team_id"] != "t-platform" {
			t.Fatalf("%s not in t-platform: %v", id, p)
		}
	}
	_, body = call(t, s, http.MethodGet, "/v1/private/people?q="+url.QueryEscape("деплоем staging"), token, "")
	if _, ok := peopleIn(body)["p-igor"]; !ok || len(body["people"].([]any)) != 1 {
		t.Fatalf("search by word forms: %v", body)
	}
	_, body = call(t, s, http.MethodGet, "/v1/private/people?q=zzzz", token, "")
	if len(body["people"].([]any)) != 0 {
		t.Fatalf("no match must be empty list: %v", body)
	}
}

func TestGetPerson(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")

	code, body := call(t, s, http.MethodGet, "/v1/private/people/p-anna", token, "")
	if code != http.StatusOK || body["person"].(map[string]any)["name"] != "Анна Белова" {
		t.Fatalf("code %d %v", code, body)
	}
	if code, _ := call(t, s, http.MethodGet, "/v1/private/people/p-ghost", token, ""); code != http.StatusNotFound {
		t.Fatalf("unknown person: code %d", code)
	}
}

func TestPeopleDirectoryOutage(t *testing.T) {
	for _, broken := range []string{"people", "person", "team"} {
		t.Run(broken, func(t *testing.T) {
			d := deps(t)
			d.Directory = brokenDirectory{Directory: d.Directory, broken: broken}
			s := newServerWith(t, d)
			token := login(t, s, "intern", "intern-demo")
			path := "/v1/private/people"
			if broken == "person" {
				path += "/p-anna"
			}
			if code, _ := call(t, s, http.MethodGet, path, token, ""); code != http.StatusServiceUnavailable {
				t.Fatalf("code %d, want 503", code)
			}
		})
	}
}

func (d brokenDirectory) People(ctx context.Context) ([]domain.Person, error) {
	if d.broken == "people" {
		return nil, errDown
	}
	return d.Directory.People(ctx)
}
