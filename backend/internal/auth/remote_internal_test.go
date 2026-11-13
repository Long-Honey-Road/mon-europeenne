package auth

import (
	"testing"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
)

func TestUserOfRejectsBadAttributes(t *testing.T) {
	for name, a := range map[string]*authrolesv1.Account{
		"role":       {UserId: "u", Role: "boss"},
		"direction":  {UserId: "u", Role: "intern", Direction: "cobol"},
		"start date": {UserId: "u", Role: "intern", StartDate: "yesterday"},
	} {
		if _, err := userOf(a); err == nil {
			t.Errorf("%s: bad value accepted", name)
		}
	}
	if u, err := userOf(&authrolesv1.Account{UserId: "u", Role: "mentor"}); err != nil || !u.StartDate.IsZero() {
		t.Errorf("minimal account = %+v, %v", u, err)
	}
}
