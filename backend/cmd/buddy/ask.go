package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/profile"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

var errNoQuestion = errors.New("usage: buddy ask [-as login] [-thread general|plan|request|task] [-detailed] <question>")

// runAsk runs one agent turn against the configured model with the tools that exist so far.
func runAsk(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	login := fs.String("as", "intern", "test account login")
	threadName := fs.String("thread", "general", "thread type")
	detailed := fs.Bool("detailed", false, "detailed answer mode")
	verbose := fs.Bool("v", false, "print tool calls and results")
	if err := fs.Parse(args); err != nil {
		return err
	}
	question := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(question) == "" {
		return errNoQuestion
	}
	thread, err := domain.ParseThreadType(*threadName)
	if err != nil {
		return err
	}

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	model, err := newLLM(cfg.LLM)
	if err != nil {
		return err
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return err
	}
	principal, err := principalFor(data, *login)
	if err != nil {
		return err
	}
	registry, closeTools, err := localTools(context.Background(), cfg, data)
	if err != nil {
		return err
	}
	defer closeTools()

	mode := agent.ModeBrief
	if *detailed {
		mode = agent.ModeDetailed
	}
	user, err := seedUsers(data).User(context.Background(), principal.UserID)
	if err != nil {
		return err
	}
	prof, err := profile.Build(context.Background(), user, data.Directory())
	if err != nil {
		return err
	}
	turn := agent.Turn{
		Principal: principal, Thread: thread, Mode: mode, Profile: prof, Message: question,
		Today: agent.TodayLine(time.Now(), user.StartDate),
	}
	var toolset agent.Tools = registry
	if *verbose {
		toolset = loggingTools{Tools: registry, w: stdout}
	}
	leased, release, err := model.Acquire(context.Background(), func() { fmt.Fprintln(stdout, "[status] В очереди…") })
	if err != nil {
		return err
	}
	defer release()
	a := agent.New(model, toolset, agent.Config{DetailedEffort: cfg.LLM.DetailedReasoningEffort, Timeout: cfg.TurnTimeout})
	res, err := a.RunWith(context.Background(), leased, turn, printer{w: stdout})
	if err != nil {
		return err
	}
	var types []string
	for _, e := range res.Events {
		types = append(types, e.Type.String())
	}
	fmt.Fprintf(stdout, "\n[events] %s\n", strings.Join(types, ", "))
	return nil
}

func principalFor(data seed.Data, login string) (domain.Principal, error) {
	for _, c := range data.Credentials() {
		if c.User.Login == login {
			return domain.Principal{UserID: c.User.ID, Role: c.User.Role, TeamID: c.User.TeamID}, nil
		}
	}
	return domain.Principal{}, fmt.Errorf("no test account %q", login)
}

// printer shows the turn as the user would see it, with markers for statuses and blocks.
type printer struct{ w io.Writer }

func (p printer) Status(text string) { fmt.Fprintf(p.w, "[%s]\n", text) }
func (p printer) Delta(text string)  { fmt.Fprint(p.w, text) }
func (p printer) Reset()             { fmt.Fprint(p.w, "\n[reset]\n") }
func (p printer) Block(b agent.Block) {
	raw, _ := json.Marshal(b)
	fmt.Fprintf(p.w, "\n[block] %s", raw)
}

// loggingTools prints every tool call with its arguments and result.
type loggingTools struct {
	agent.Tools
	w io.Writer
}

func (l loggingTools) Call(ctx context.Context, p domain.Principal, thread domain.ThreadType, name string, args json.RawMessage) (json.RawMessage, error) {
	out, err := l.Tools.Call(ctx, p, thread, name, args)
	fmt.Fprintf(l.w, "[tool] %s %s -> %s err=%v\n", name, args, out, err)
	return out, err
}
