package domain

import "io/fs"

// FileNode is the projection of a filesystem entry that the agent reasons
// about. It carries the metadata (kind, mode, owner, size) needed to choose
// a sync action. The content hash is fetched on demand via SourceReader /
// TargetWriter rather than stored on the node, so a Stat doesn't pay for a
// SHA-256 unless we actually need it.
type FileNode struct {
	Kind NodeKind
	Mode fs.FileMode
	UID  int
	GID  int
	Size int64
}
