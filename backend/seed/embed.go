package seed

import "embed"

// Files is the shipped synthetic data set.
//
//go:embed *.json
var Files embed.FS
