// Package teamrepo reads team repositories through the team-repo contract: the mock today, Bitbucket later.
package teamrepo

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	teamrepov1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/teamrepo/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

type Client struct {
	c teamrepov1.TeamRepoServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{c: teamrepov1.NewTeamRepoServiceClient(conn)}
}

func wrap(err error) error {
	switch status.Code(err) {
	case codes.NotFound:
		return fmt.Errorf("%w: %s", tools.ErrNotFound, status.Convert(err).Message())
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", tools.ErrIsDir, status.Convert(err).Message())
	default:
		return fmt.Errorf("team-repo: %w", err)
	}
}

func (c *Client) Tree(ctx context.Context, team, path string, depth int) (string, []tools.RepoEntry, error) {
	resp, err := c.c.GetTree(ctx, &teamrepov1.GetTreeRequest{TeamId: team, Path: path, Depth: int32(depth)}) //nolint:gosec // depth is 1..3
	if err != nil {
		return "", nil, wrap(err)
	}
	out := make([]tools.RepoEntry, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		out = append(out, tools.RepoEntry{Path: e.GetPath(), Dir: e.GetDir()})
	}
	return resp.GetRepo(), out, nil
}

func (c *Client) File(ctx context.Context, team, path string) (string, string, error) {
	resp, err := c.c.GetFile(ctx, &teamrepov1.GetFileRequest{TeamId: team, Path: path})
	if err != nil {
		return "", "", wrap(err)
	}
	return resp.GetRepo(), resp.GetText(), nil
}

func (c *Client) Process(ctx context.Context, team string) (string, []tools.ProcessSection, error) {
	resp, err := c.c.GetProcess(ctx, &teamrepov1.GetProcessRequest{TeamId: team})
	if err != nil {
		return "", nil, wrap(err)
	}
	out := make([]tools.ProcessSection, 0, len(resp.GetSections()))
	for _, s := range resp.GetSections() {
		out = append(out, tools.ProcessSection{Key: s.GetKey(), Title: s.GetTitle(), Markdown: s.GetMarkdown(), File: s.GetFile()})
	}
	return resp.GetRepo(), out, nil
}
