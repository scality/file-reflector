package matcher

import (
	"path/filepath"
	"strings"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrInvalidPattern is the sentinel returned by NewGlob when one of the
// supplied patterns is rejected by filepath.Match.
var ErrInvalidPattern = errors.New("invalid ignore pattern")

// Glob implements service.Matcher with gitignore-lite semantics:
//
//   - Patterns without "/" are matched against every component of the
//     candidate path. So "cache" ignores both the "cache" entry itself
//     and anything below it (any path that has "cache" as one of its
//     segments), and "*.tmp" ignores every .tmp file or .tmp-suffixed
//     segment anywhere in the tree.
//   - Patterns with "/" are matched against the relative path from the
//     target root via filepath.Match (anchored, no implicit "**"
//     recursion).
type Glob struct {
	patterns []compiled
}

type compiled struct {
	raw      string
	hasSlash bool
}

// Compile-time interface assertion.
var _ service.Matcher = (*Glob)(nil)

// NewGlob compiles a list of patterns into a Glob matcher. Returns
// ErrInvalidPattern (wrapped) if any pattern is syntactically invalid
// for filepath.Match.
func NewGlob(patterns []string) (*Glob, error) {
	out := make([]compiled, 0, len(patterns))

	for _, p := range patterns {
		if _, err := filepath.Match(p, "validate"); err != nil {
			return nil, errors.Wrap(ErrInvalidPattern,
				errors.WithProperty("pattern", p),
				errors.CausedBy(err),
			)
		}

		out = append(out, compiled{raw: p, hasSlash: strings.ContainsRune(p, '/')})
	}

	return &Glob{patterns: out}, nil
}

// Matches reports whether rel matches any of the configured patterns.
func (g *Glob) Matches(rel string) bool {
	if g == nil || len(g.patterns) == 0 {
		return false
	}

	components := strings.Split(rel, "/")

	for _, p := range g.patterns {
		if p.hasSlash {
			if ok, _ := filepath.Match(p.raw, rel); ok {
				return true
			}

			continue
		}

		for _, c := range components {
			if ok, _ := filepath.Match(p.raw, c); ok {
				return true
			}
		}
	}

	return false
}
