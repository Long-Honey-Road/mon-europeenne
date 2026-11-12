package store

import (
	"os"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

func freshDB(t *testing.T) string {
	return testdb.Fresh(t)
}
