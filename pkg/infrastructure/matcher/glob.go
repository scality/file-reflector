package matcher

import "github.com/scality/file-reflector/pkg/service"

// Glob implements service.Matcher with gitignore-lite semantics:
// patterns without "/" match the basename of the candidate path at any
// depth; patterns with "/" match the relative path from the target root.
type Glob struct {
	patterns []string
}

// Compile-time interface assertion.
var _ service.Matcher = (*Glob)(nil)

// NewGlob compiles a list of patterns into a Glob matcher. Returns an
// error if any pattern is syntactically invalid for filepath.Match.
func NewGlob(patterns []string) (*Glob, error) {
	return &Glob{patterns: patterns}, nil
}

// Matches reports whether rel matches any of the configured patterns.
//
// TODO: real implementation. The skeleton returns false.
func (g *Glob) Matches(_ string) bool {
	return false
}
