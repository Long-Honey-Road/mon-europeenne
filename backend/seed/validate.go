package seed

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Approver roles a person can hold for request chains besides their manager or an object owner.
var approverRoles = []string{"hr_admin", "devops"}

// problems collects every inconsistency so one run shows all of them.
type problems []string

func (p *problems) addf(format string, args ...any) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

func validate(d Data) problems {
	var p problems
	persons := validatePeople(&p, d.People)
	validateRequests(&p, d.Requests, d.People, persons)
	validatePlan(&p, d.Plan, d.People)
	return p
}

// validatePeople returns the set of known person ids.
func validatePeople(p *problems, f PeopleFile) map[string]bool {
	teams := map[string]bool{}
	for _, t := range f.Teams {
		if teams[t.ID] {
			p.addf("team %q: duplicate id", t.ID)
		}
		teams[t.ID] = true
	}

	persons := map[string]bool{}
	for _, person := range f.People {
		if persons[person.ID] {
			p.addf("person %q: duplicate id", person.ID)
		}
		persons[person.ID] = true
		if !teams[person.TeamID] {
			p.addf("person %q: unknown team %q", person.ID, person.TeamID)
		}
		for _, role := range person.ApproverFor {
			if !slices.Contains(approverRoles, role) {
				p.addf("person %q: unknown approver role %q", person.ID, role)
			}
		}
	}

	logins, users := map[string]bool{}, map[string]bool{}
	for _, person := range f.People {
		a := person.Account
		if a == nil {
			continue
		}
		if logins[a.Login] || users[a.UserID] {
			p.addf("person %q: duplicate login or user id", person.ID)
		}
		logins[a.Login], users[a.UserID] = true, true
		if a.Login == "" || a.Password == "" || a.UserID == "" {
			p.addf("person %q: account needs user_id, login and password", person.ID)
		}
		if _, err := domain.ParseRole(a.Role); err != nil {
			p.addf("person %q: %v", person.ID, err)
		}
		if a.Direction != "" {
			if _, err := domain.ParseDirection(a.Direction); err != nil {
				p.addf("person %q: %v", person.ID, err)
			}
		}
		for _, ref := range []string{a.MentorID, a.ManagerID, a.HRBPID} {
			if ref != "" && !persons[ref] {
				p.addf("person %q: relation to unknown person %q", person.ID, ref)
			}
		}
		if a.StartDate != "" {
			if _, err := time.Parse(time.DateOnly, a.StartDate); err != nil {
				p.addf("person %q: start_date %q is not YYYY-MM-DD", person.ID, a.StartDate)
			}
		}
	}
	return persons
}

func validateRequests(p *problems, f RequestsFile, pf PeopleFile, persons map[string]bool) {
	objectKinds := map[string]bool{}
	for _, o := range f.Objects {
		objectKinds[o.Kind] = true
		if !persons[o.OwnerID] {
			p.addf("object %s/%s: unknown owner %q", o.Kind, o.Key, o.OwnerID)
		}
	}
	approvers := map[string]bool{}
	for _, person := range pf.People {
		for _, role := range person.ApproverFor {
			approvers[role] = true
		}
	}

	keys := map[string]bool{}
	for _, t := range f.Types {
		if keys[t.Key] {
			p.addf("request type %q: duplicate key", t.Key)
		}
		keys[t.Key] = true
		if err := checkFieldsSchema(t.FieldsSchema); err != nil {
			p.addf("request type %q: fields_schema: %v", t.Key, err)
		}
		if len(t.ApprovalChain) == 0 {
			p.addf("request type %q: empty approval chain", t.Key)
		}
		for _, a := range t.ApprovalChain {
			switch a.Resolver {
			case "manager":
			case "object_owner":
				if !objectKinds[a.Object] {
					p.addf("request type %q: no objects of kind %q to resolve an owner", t.Key, a.Object)
				}
			case "hr_admin", "devops":
				if !approvers[a.Resolver] {
					p.addf("request type %q: nobody is approver_for %q", t.Key, a.Resolver)
				}
			default:
				p.addf("request type %q: unknown resolver %q", t.Key, a.Resolver)
			}
		}
		switch t.Demo.Outcome {
		case "approve":
		case "reject":
			if t.Demo.Reason == "" {
				p.addf("request type %q: demo reject needs a reason", t.Key)
			}
		default:
			p.addf("request type %q: unknown demo outcome %q", t.Key, t.Demo.Outcome)
		}
	}
}

func checkFieldsSchema(raw json.RawMessage) error {
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Type != "object" {
		return fmt.Errorf("type must be object, got %q", s.Type)
	}
	_, err := s.Resolve(nil)
	return err
}

func validatePlan(p *problems, f PlanFile, pf PeopleFile) {
	teams := map[string]bool{}
	for _, t := range pf.Teams {
		teams[t.ID] = true
	}
	users := map[string]bool{}
	for _, person := range pf.People {
		if person.Account != nil {
			users[person.Account.UserID] = true
		}
	}

	items := map[string]PlanItem{}
	for _, it := range f.Items {
		if _, dup := items[it.Key]; dup {
			p.addf("plan item %q: duplicate key", it.Key)
		}
		items[it.Key] = it
		if _, err := domain.ParsePlanPeriod(it.Period); err != nil {
			p.addf("plan item %q: %v", it.Key, err)
		}
		if it.Direction != "" {
			if _, err := domain.ParseDirection(it.Direction); err != nil {
				p.addf("plan item %q: %v", it.Key, err)
			}
		}
		if it.TeamID != "" && !teams[it.TeamID] {
			p.addf("plan item %q: unknown team %q", it.Key, it.TeamID)
		}
	}
	for _, it := range f.Items {
		for _, dep := range it.DependsOn {
			if _, ok := items[dep]; !ok {
				p.addf("plan item %q: depends on unknown item %q", it.Key, dep)
			}
		}
	}
	if cycle := findCycle(items); cycle != "" {
		p.addf("plan: dependency cycle through %q", cycle)
	}

	for _, s := range f.Statuses {
		if !users[s.UserID] {
			p.addf("plan status: unknown user %q", s.UserID)
		}
		if _, ok := items[s.ItemKey]; !ok {
			p.addf("plan status: unknown item %q", s.ItemKey)
		}
		if _, err := domain.ParsePlanItemStatus(s.Status); err != nil {
			p.addf("plan status %s/%s: %v", s.UserID, s.ItemKey, err)
		}
	}
}

// findCycle returns a key on a dependency cycle, or "" if the plan is a DAG.
func findCycle(items map[string]PlanItem) string {
	const (
		unseen = iota
		inStack
		done
	)
	state := map[string]int{}
	var visit func(key string) string
	visit = func(key string) string {
		switch state[key] {
		case inStack:
			return key
		case done:
			return ""
		}
		state[key] = inStack
		for _, dep := range items[key].DependsOn {
			if _, ok := items[dep]; !ok {
				continue
			}
			if c := visit(dep); c != "" {
				return c
			}
		}
		state[key] = done
		return ""
	}
	for key := range items {
		if c := visit(key); c != "" {
			return c
		}
	}
	return ""
}
