package service

// Matcher decides whether a path (relative to the target root) is ignored.
// It is consulted only on the exact path the agent is about to touch,
// never recursively on ancestors.
type Matcher interface {
	Matches(relPath string) bool
}
