package seed

import (
	"slices"
	"strings"
	"testing"
)

// The shipped data must carry everything the demo scenarios rely on.
func TestShippedDataSupportsScenarios(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatalf("Load(Files): %v", err)
	}

	accounts := map[string]*Account{}
	for _, p := range d.People.People {
		if p.Account != nil {
			accounts[p.Account.Role] = p.Account
		}
	}
	intern, manager := accounts["intern"], accounts["manager"]
	if intern == nil || manager == nil {
		t.Fatal("need intern and manager accounts for the rights demo")
	}
	if intern.MentorID == "" || intern.ManagerID == "" || intern.HRBPID == "" {
		t.Errorf("intern needs the full escalation chain: %+v", intern)
	}

	teamOf := map[string]string{}
	for _, p := range d.People.People {
		teamOf[p.ID] = p.TeamID
	}
	clusterOf := map[string]string{}
	for _, team := range d.People.Teams {
		clusterOf[team.ID] = team.Cluster
	}
	internTeam := teamOf[findPersonByUser(d, intern.UserID)]
	sameTeam, otherCluster := 0, 0
	for _, p := range d.People.People {
		if p.TeamID == internTeam {
			sameTeam++
		}
		if clusterOf[p.TeamID] != clusterOf[internTeam] {
			otherCluster++
		}
	}
	if sameTeam < 6 || sameTeam > 8 {
		t.Errorf("intern's team has %d people, want 6..8", sameTeam)
	}
	if otherCluster == 0 {
		t.Error("need someone outside the intern's cluster for 'not our zone'")
	}

	var access []RequestType
	for _, rt := range d.Requests.Types {
		if strings.HasPrefix(rt.Key, "access.") {
			access = append(access, rt)
		}
	}
	if len(access) != 7 {
		t.Errorf("%d access types, want 7 for S1", len(access))
	}
	rejected := 0
	for _, rt := range access {
		if rt.Demo.Outcome == "reject" {
			rejected++
		}
	}
	if rejected != 1 {
		t.Errorf("%d access types rejected in the demo, want exactly 1", rejected)
	}

	dayOff := findType(d, "absence.day_off")
	if dayOff == nil {
		t.Fatal("absence.day_off is the S2 demo type")
	}
	var chain []string
	for _, a := range dayOff.ApprovalChain {
		chain = append(chain, a.Resolver)
	}
	if !slices.Equal(chain, []string{"manager", "hr_admin"}) {
		t.Errorf("day off chain = %v, want manager then hr_admin", chain)
	}

	periods := map[string]bool{}
	for _, it := range d.Plan.Items {
		periods[it.Period] = true
	}
	for _, period := range []string{"w1", "w2", "m1", "m2", "equator", "final"} {
		if !periods[period] {
			t.Errorf("plan has no items in period %s", period)
		}
	}
	if len(d.Plan.Statuses) == 0 {
		t.Error("intern needs some progress so the Today panel is not empty")
	}
}

func findPersonByUser(d Data, userID string) string {
	for _, p := range d.People.People {
		if p.Account != nil && p.Account.UserID == userID {
			return p.ID
		}
	}
	return ""
}

func findType(d Data, key string) *RequestType {
	for i := range d.Requests.Types {
		if d.Requests.Types[i].Key == key {
			return &d.Requests.Types[i]
		}
	}
	return nil
}
