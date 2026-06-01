package di

import (
	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/matcher"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrBuildMatcher is the sentinel returned when the container cannot
// build its Matcher singleton.
var ErrBuildMatcher = errors.New("build matcher")

// getMatcher returns the singleton Matcher built from the --ignore
// patterns. Returns an error if any pattern is syntactically invalid.
func (c *Container) getMatcher() (service.Matcher, error) {
	if c.matcher == nil {
		m, err := matcher.NewGlob(c.cfg.Ignore)
		if err != nil {
			return nil, errors.Wrap(ErrBuildMatcher, errors.CausedBy(err))
		}

		c.matcher = m
	}

	return c.matcher, nil
}
