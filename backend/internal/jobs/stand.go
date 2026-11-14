package jobs

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
)

// maxStandRules keeps a stand's rules a short addition to the prompt.
const maxStandRules = 16 << 10

// FileRules reads a stand's prompt rules from path on every call, so edits apply without a restart. A missing or
// unreadable file means no rules: the turn goes on as usual.
func FileRules(path string) func() string {
	return func() string {
		f, err := os.Open(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("stand rules", "file", path, "err", err)
			}
			return ""
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, maxStandRules))
		if err != nil {
			slog.Warn("stand rules", "file", path, "err", err)
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}
