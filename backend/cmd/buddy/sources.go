package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mockconn"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests/corpclient"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/teamrepo"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// directoryTTL is how long a copy of the people directory is used before it is read again.
const directoryTTL = time.Minute

const corpCursorKey = "corp-requests:cursor"

var errNoMocks = errors.New("FEATURE_REQUESTS needs MOCKS_ADDR: access requests go to the corp-requests service")

// sources are the corporate services buddy reads: over gRPC when MOCKS_ADDR is set (the mocks today, real
// services later), seed in memory otherwise, for tests and local runs without compose.
type sources struct {
	accounts  auth.Directory
	directory people.Directory
	conn      *grpc.ClientConn
}

func openSources(cfg config.Config, data seed.Data) (sources, func(), error) {
	if cfg.MocksAddr == "" {
		accounts, err := auth.NewAccounts(data.Credentials())
		if err != nil {
			return sources{}, nil, err
		}
		return sources{accounts: accounts, directory: data.Directory()}, func() {}, nil
	}
	conn, err := mockconn.Dial(cfg.MocksAddr, mockconn.DefaultTimeout)
	if err != nil {
		return sources{}, nil, fmt.Errorf("MOCKS_ADDR: %w", err)
	}
	src := sources{accounts: auth.NewRemote(conn), directory: people.NewRemote(conn, directoryTTL), conn: conn}
	return src, func() { _ = conn.Close() }, nil
}

// teamRepo is the team repository host, or nil without MOCKS_ADDR (no repository tools then).
func (s sources) teamRepo() tools.TeamRepo {
	if s.conn == nil {
		return nil
	}
	return teamrepo.New(s.conn)
}

// requestService is the company's request system; there is no in-memory stand-in, requests need the service.
func (s sources) requestService(rdb *redis.Client) (requests.Service, error) {
	if s.conn == nil {
		return nil, errNoMocks
	}
	return corpclient.New(s.conn, corpclient.NewRedisCursor(rdb, corpCursorKey)), nil
}
