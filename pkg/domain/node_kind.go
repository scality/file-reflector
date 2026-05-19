package domain

// NodeKind identifies what kind of filesystem entry a FileNode describes.
// The empty string is not a valid value; constructors must set one of the
// named constants below.
type NodeKind string

const (
	NodeAbsent  NodeKind = "absent"
	NodeFile    NodeKind = "file"
	NodeDir     NodeKind = "dir"
	NodeSymlink NodeKind = "symlink"
)

// String returns the kind as a human-readable string.
func (nk NodeKind) String() string {
	return string(nk)
}
