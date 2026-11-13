package corpclient_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests/corpclient"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
)

func TestMain(m *testing.M) { os.Exit(testredis.Main(m)) }

type memCursor struct {
	v       int64
	loadErr error
	saveErr error
}

func (c *memCursor) Load(context.Context) (int64, error) { return c.v, c.loadErr }
func (c *memCursor) Save(_ context.Context, v int64) error {
	if c.saveErr != nil {
		return c.saveErr
	}
	c.v = v
	return nil
}

var sub = requests.Submission{
	TypeKey: "access.jira_project", RequesterID: "u-liza",
	Fields:    map[string]any{"project_key": "MSG", "level": "read"},
	Approvers: []requests.Approver{{Role: "object_owner", PersonID: "p-oleg", Name: "Олег"}, {Role: "manager", PersonID: "p-dmitry", Name: "Дмитрий"}},
}

func TestSubmitAndUpdates(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	cur := &memCursor{}
	c := corpclient.New(conn, cur)
	ctx := t.Context()

	tk, err := c.Submit(ctx, sub)
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID != "REQ-1" || tk.Status != requests.StatusPending || tk.Fields["level"] != "read" || len(tk.Approvers) != 2 ||
		tk.Approver().Name != "Олег" || len(tk.History) != 1 || tk.UpdatedAt.IsZero() {
		t.Fatalf("ticket = %+v", tk)
	}

	got, err := c.Updates(ctx)
	if err != nil || len(got) != 1 || cur.v == 0 {
		t.Fatalf("first Updates = %v, %v; cursor %d", got, err, cur.v)
	}
	if again, _ := c.Updates(ctx); len(again) != 0 {
		t.Errorf("Updates without changes = %d tickets", len(again))
	}

	srv.Requests.Tick(time.Now().Add(time.Hour))
	srv.Requests.Tick(time.Now().Add(2 * time.Hour))
	got, _ = c.Updates(ctx)
	if len(got) != 1 || got[0].Status != requests.StatusApproved || got[0].Approver().Name != "Дмитрий" || len(got[0].History) != 3 {
		t.Fatalf("after decisions = %+v", got)
	}
}

func TestSubmitErrors(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	c := corpclient.New(conn, &memCursor{})
	if _, err := c.Submit(t.Context(), requests.Submission{TypeKey: "access.vpn", RequesterID: "u-liza"}); status.Code(errors.Unwrap(err)) != codes.InvalidArgument {
		t.Errorf("submit without approvers: %v", err)
	}
	bad := sub
	bad.Fields = map[string]any{"x": make(chan int)}
	if _, err := c.Submit(t.Context(), bad); err == nil {
		t.Error("unencodable fields accepted")
	}
}

func TestUpdatesKeepsCursorOnFailure(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{FailRate: 1})
	cur := &memCursor{v: 7}
	if _, err := corpclient.New(conn, cur).Updates(t.Context()); err == nil || cur.v != 7 {
		t.Errorf("Updates on a failing service = %v, cursor %d", err, cur.v)
	}

	ok, _ := mockstest.Start(t, mocks.Chaos{})
	if _, err := corpclient.New(ok, &memCursor{loadErr: errors.New("down")}).Updates(t.Context()); err == nil {
		t.Error("cursor load error ignored")
	}
	c := corpclient.New(ok, &memCursor{saveErr: errors.New("down")})
	if _, err := c.Submit(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Updates(t.Context()); err == nil || len(got) != 1 {
		t.Errorf("cursor save error = %v with %d tickets, want the error and the tickets", err, len(got))
	}
}

func TestRedisCursor(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: testredis.Addr(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	cur := corpclient.NewRedisCursor(rdb, "test:corpclient:"+t.Name())
	ctx := t.Context()
	if v, err := cur.Load(ctx); v != 0 || err != nil {
		t.Fatalf("empty Load = %d, %v", v, err)
	}
	if err := cur.Save(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if v, err := cur.Load(ctx); v != 42 || err != nil {
		t.Errorf("Load = %d, %v", v, err)
	}
	_ = rdb.Set(ctx, "test:corpclient:bad", "x", 0).Err()
	if _, err := corpclient.NewRedisCursor(rdb, "test:corpclient:bad").Load(ctx); err == nil {
		t.Error("garbage cursor accepted")
	}
	_ = rdb.Close()
	if _, err := cur.Load(ctx); err == nil {
		t.Error("closed client: no error")
	}
}
