package main

import (
	"fmt"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// fakeAnswer is what LLM_PROVIDER=fake says to every question; it names the mode so nobody mistakes it for buddy.
const fakeAnswer = "Тестовый режим: модель не подключена (LLM_PROVIDER=fake). Вопрос получен, ответ придёт, когда подключат модель."

// agentTools is the tool set available today; A-05 adds plan, requests and repository tools.
func agentTools(data seed.Data) (*tools.Registry, error) {
	who, err := tools.WhoToAsk(data.Directory())
	if err != nil {
		return nil, err
	}
	notFound, err := tools.ReportNotFound(data.Directory())
	if err != nil {
		return nil, err
	}
	return tools.NewRegistry(who, notFound)
}

// seedUsers looks up test users by id without hashing passwords.
type seedUsers seed.Data

func (s seedUsers) User(id string) (domain.User, error) {
	for _, c := range seed.Data(s).Credentials() {
		if c.User.ID == id {
			return c.User, nil
		}
	}
	return domain.User{}, fmt.Errorf("unknown user %q", id)
}
