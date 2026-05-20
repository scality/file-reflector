package config

import "io/fs"

// Owner is the uid:gid pair the agent applies to synced entries when the
// operator explicitly sets --owner. A nil *Owner means "preserve the
// source's uid/gid".
type Owner struct {
	UID int
	GID int
}

// Config is the parsed command-line configuration passed from the
// composition root into the DI container. Optional flags use pointer
// types so a nil value distinguishes "not set" (preserve source) from
// "explicitly set to zero".
type Config struct {
	Source string
	Target string

	Ignore []string

	FileMode *fs.FileMode
	DirMode  *fs.FileMode
	Owner    *Owner

	LogFormat string // "text" | "json"
	LogLevel  string // "debug" | "info" | "warn" | "error"
}
