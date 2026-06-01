package service

// SourceReader exposes read-only access to the source filesystem tree.
// The empty string ("") refers to the source root; every other path is
// relative to that root.
type SourceReader interface {
	MetadataReader
	ContentReader
	ContentHasher
}
