package seed

import "embed"

// Files is the shipped synthetic data set.
//
//go:embed *.json docs/*.md guide/*.md all:_repo
var Files embed.FS
