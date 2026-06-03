package contenthasher

import (
	"context"
	"crypto/sha256"
	"io"
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrContentHash is the sentinel returned for any failure hashing
// filesystem entry content.
var ErrContentHash = errors.New("content hash")

// OSFS implements service.ContentHasher against the local OS
// filesystem, rooted at a single directory.
type OSFS struct {
	root *os.Root
}

// Compile-time interface assertion.
var _ service.ContentHasher = (*OSFS)(nil)

// NewOSFS returns an OSFS content hasher rooted at root.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// Hash returns the SHA-256 of the file content at rel.
func (o *OSFS) Hash(_ context.Context, rel string) ([32]byte, error) {
	f, err := o.root.Open(rel)
	if err != nil {
		return [32]byte{}, errors.Wrap(ErrContentHash,
			errors.WithDetail("open for hash"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, errors.Wrap(ErrContentHash,
			errors.WithDetail("hash"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	var out [32]byte
	copy(out[:], h.Sum(nil))

	return out, nil
}
