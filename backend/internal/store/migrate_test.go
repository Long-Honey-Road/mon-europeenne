package store

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var wantTables = []string{
	"chunks", "conflict_statuses", "conflicts", "directions", "documents", "event_types", "events", "focus_kinds", "focus_summaries",
	"message_roles", "messages", "people", "pins", "plan_item_dependencies", "plan_item_statuses", "plan_items", "plan_periods",
	"plan_status", "requests", "roles", "source_kinds", "teams", "thread_types", "threads", "turn_traces", "users",
}

func TestMigrateUpCreatesSchemaFromScratch(t *testing.T) {
	dsn := freshDB(t)

	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	if got := tables(t, dsn); !slices.Equal(got, wantTables) {
		t.Fatalf("tables after up =\n%v\nwant\n%v", got, wantTables)
	}
}

func TestMigrateUpTwiceIsNoop(t *testing.T) {
	dsn := freshDB(t)

	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("second MigrateUp: %v", err)
	}
}

func TestMigrateDownRemovesEverything(t *testing.T) {
	dsn := freshDB(t)
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	if err := MigrateDown(dsn); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}

	if got := tables(t, dsn); len(got) != 0 {
		t.Fatalf("tables after down = %v, want none", got)
	}
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp after down: %v", err)
	}
}

func TestLookupTablesMatchDomainEnums(t *testing.T) {
	dsn := freshDB(t)
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	conn := connect(t, dsn)

	for table, want := range domain.Lookups() {
		rows, err := conn.Query(context.Background(), "SELECT id, name FROM buddy."+table)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		got := map[int16]string{}
		for rows.Next() {
			var id int16
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			got[id] = name
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows %s: %v", table, err)
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s = %v, want %v", table, got, want)
		}
	}
}

func TestMigrateRejectsBadDSN(t *testing.T) {
	if err := MigrateUp("mysql://nope"); err == nil {
		t.Fatal("MigrateUp with non-postgres DSN succeeded")
	}
	if err := MigrateDown("mysql://nope"); err == nil {
		t.Fatal("MigrateDown with non-postgres DSN succeeded")
	}
}

func tables(t *testing.T, dsn string) []string {
	t.Helper()
	rows, err := connect(t, dsn).Query(context.Background(),
		`SELECT table_name FROM information_schema.tables WHERE table_schema = 'buddy' ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect tables: %v", err)
	}
	return names
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	return conn
}
