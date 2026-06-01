package service

// TargetWriter exposes the read + write surface needed to make the target
// tree reflect the source tree. The empty string ("") refers to the target
// root; every other path is relative to that root.
type TargetWriter interface {
	MetadataReader
	ContentHasher
	ContentWriter
	MetadataWriter
	EntryRemover
}
