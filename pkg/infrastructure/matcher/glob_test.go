package matcher_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/matcher"
)

var _ = Describe("Glob", func() {
	Describe("NewGlob", func() {
		It("returns an error that matches ErrInvalidPattern on a malformed pattern", func() {
			_, err := matcher.NewGlob([]string{"[unterminated"})

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, matcher.ErrInvalidPattern)).To(BeTrue())
		})
	})

	DescribeTable("Matches",
		func(patterns []string, candidate string, expected bool) {
			m, err := matcher.NewGlob(patterns)
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Matches(candidate)).To(Equal(expected))
		},

		Entry("no patterns, simple path", nil, "anything", false),
		Entry("no patterns, deep path", nil, "a/b", false),

		// Basename rule: pattern without "/" matches the basename of the
		// candidate at any depth.
		Entry("basename star-ext at root", []string{"*.tmp"}, "foo.tmp", true),
		Entry("basename star-ext deep", []string{"*.tmp"}, "a/b/c/foo.tmp", true),
		Entry("basename star-ext no match", []string{"*.tmp"}, "foo.txt", false),
		Entry("basename literal", []string{"cache"}, "cache", true),
		Entry("basename literal at depth", []string{"cache"}, "a/cache", true),
		Entry("basename literal matches intermediate component", []string{"cache"}, "a/cache/inner", true),
		Entry("basename literal no match anywhere", []string{"cache"}, "a/other/inner", false),
		Entry("basename wildcard matches intermediate dir-like component", []string{"*.tmp"}, "a/foo.tmp/inner", true),

		// Anchored rule: pattern with "/" matches the relative path from
		// the target root via filepath.Match.
		Entry("anchored exact", []string{"a/b"}, "a/b", true),
		Entry("anchored does not match unanchored", []string{"a/b"}, "x/a/b", false),
		Entry("anchored wildcard segment", []string{"subdir/*.log"}, "subdir/app.log", true),
		Entry("anchored wildcard does not cross dirs", []string{"subdir/*.log"}, "subdir/inner/app.log", false),

		// Multiple patterns: ANY match → ignored.
		Entry("multiple patterns, one match", []string{"*.tmp", "secrets"}, "secrets", true),
		Entry("multiple patterns, no match", []string{"*.tmp", "secrets"}, "data.txt", false),

		// Character classes work, courtesy of filepath.Match.
		Entry("char class match", []string{"[Tt]emp"}, "Temp", true),
		Entry("char class mismatch", []string{"[Tt]emp"}, "Pemp", false),

		// Glob is exact-match by default: no implicit substring, prefix, or
		// suffix matching unless an explicit wildcard is in the pattern.
		Entry("literal does not match substring", []string{"oto"}, "toto", false),
		Entry("literal does not match substring in segment", []string{"oto"}, "abc/toto/data.txt", false),
		Entry("literal does not match suffix without wildcard", []string{"toto"}, "foototo", false),
		Entry("wildcard prefix matches suffix", []string{"*toto"}, "foototo", true),
		Entry("? matches exactly one character", []string{"?ile"}, "file", true),
		Entry("? is length-strict, does not match longer", []string{"?ile"}, "files", false),
	)
})
