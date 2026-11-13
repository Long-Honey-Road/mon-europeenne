package seed

import "embed"

// Files is the shipped synthetic data set.
//
//go:embed *.json docs/*.md
var Files embed.FS
