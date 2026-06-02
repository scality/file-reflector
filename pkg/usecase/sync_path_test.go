package usecase_test

import (
	"context"
	stderrors "errors"
	"io"
	"io/fs"
	"log/slog"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"
	"github.com/stretchr/testify/mock"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service/mocks"
	"github.com/scality/file-reflector/pkg/usecase"
)

var _ = Describe("SyncPath", func() {
	const path = "rel/file"

	var (
		logger      *slog.Logger
		sourceMock  *mocks.MockSourceReader
		targetMock  *mocks.MockTargetWriter
		matcherMock *mocks.MockMatcher
		uc          *usecase.SyncPath
		ctx         context.Context
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		sourceMock = mocks.NewMockSourceReader(GinkgoT())
		targetMock = mocks.NewMockTargetWriter(GinkgoT())
		matcherMock = mocks.NewMockMatcher(GinkgoT())
		uc = usecase.NewSyncPath(logger, sourceMock, targetMock, matcherMock)
		ctx = context.Background()
	})

	Describe("when relPath is the watched root", func() {
		It("is a no-op without consulting the matcher or any side", func() {
			Expect(uc.Execute(ctx, "")).To(Succeed())
		})
	})

	Describe("when the path matches the ignore matcher", func() {
		It("is a no-op", func() {
			matcherMock.EXPECT().Matches(path).Return(true)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})
	})

	Describe("when source is a symlink", func() {
		BeforeEach(func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeSymlink}, nil)
		})

		It("is a no-op when target is also absent", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes a stale target entry", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})
	})

	Describe("when source is absent", func() {
		BeforeEach(func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
		})

		It("is a no-op when target is also absent", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target when it exists", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target directory when target is a directory", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeDir}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})
	})

	Describe("when source is a regular file", func() {
		var src domain.FileNode

		BeforeEach(func() {
			src = domain.FileNode{
				Kind: domain.NodeFile,
				Mode: 0o644,
				UID:  1000,
				GID:  1000,
				Size: 10,
			}

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
		})

		It("writes the target file when it does not exist", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("payload")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o644), 1000, 1000).
				Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("is a no-op when content and metadata both match", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)

			hash := [32]byte{0xAB}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("chmods target when content matches but mode differs", func() {
			tgt := src
			tgt.Mode = 0o600
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0xCD}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chmod(mock.Anything, path, fs.FileMode(0o644)).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("chowns target when content matches but uid differs", func() {
			tgt := src
			tgt.UID = 2000
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0x12}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chown(mock.Anything, path, 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("chowns target when content matches but gid differs", func() {
			tgt := src
			tgt.GID = 2000
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0x34}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chown(mock.Anything, path, 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("chmods and chowns target when both mode and owner differ", func() {
			tgt := src
			tgt.Mode = 0o600
			tgt.UID = 2000
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0x56}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chmod(mock.Anything, path, fs.FileMode(0o644)).Return(nil)
			targetMock.EXPECT().Chown(mock.Anything, path, 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("rewrites without hashing when target size differs", func() {
			tgt := src
			tgt.Size = 99
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("payload")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o644), 1000, 1000).
				Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("rewrites when sizes match but hashes differ", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			sourceMock.EXPECT().Hash(mock.Anything, path).Return([32]byte{0xAA}, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return([32]byte{0xBB}, nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("payload")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o644), 1000, 1000).
				Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target dir then writes file when target is a directory", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeDir}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("payload")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o644), 1000, 1000).
				Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target symlink then writes file when target is a symlink", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeSymlink}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("payload")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o644), 1000, 1000).
				Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})
	})

	Describe("when source is a directory", func() {
		var src domain.FileNode

		BeforeEach(func() {
			src = domain.FileNode{
				Kind: domain.NodeDir,
				Mode: 0o750,
				UID:  1000,
				GID:  1000,
			}

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
		})

		It("creates target dir when it does not exist", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			targetMock.EXPECT().
				Mkdir(mock.Anything, path, fs.FileMode(0o750), 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("is a no-op when target dir has matching metadata", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("syncs metadata when target dir has different mode", func() {
			tgt := src
			tgt.Mode = 0o755
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)
			targetMock.EXPECT().Chmod(mock.Anything, path, fs.FileMode(0o750)).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("syncs ownership when target dir has different gid", func() {
			tgt := src
			tgt.GID = 2000
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)
			targetMock.EXPECT().Chown(mock.Anything, path, 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target file then creates dir when target was a file", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)
			targetMock.EXPECT().
				Mkdir(mock.Anything, path, fs.FileMode(0o750), 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})

		It("removes target symlink then creates dir when target was a symlink", func() {
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeSymlink}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).Return(nil)
			targetMock.EXPECT().
				Mkdir(mock.Anything, path, fs.FileMode(0o750), 1000, 1000).Return(nil)

			Expect(uc.Execute(ctx, path)).To(Succeed())
		})
	})

	Describe("error propagation", func() {
		It("wraps ErrSyncPath when source stat fails", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{}, stderrors.New("disk dead"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when target write fails", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile, Mode: 0o600, Size: 1}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(io.NopCloser(strings.NewReader("x")), nil)
			targetMock.EXPECT().
				WriteAtomic(mock.Anything, path, mock.Anything, fs.FileMode(0o600), 0, 0).
				Return(stderrors.New("ENOSPC"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when target stat fails", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile, Size: 1}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{}, stderrors.New("disk dead"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when Remove fails on an absent source", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).
				Return(stderrors.New("EBUSY"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when Remove fails on a wrong-kind target", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeFile, Size: 1}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeDir}, nil)
			targetMock.EXPECT().Remove(mock.Anything, path).
				Return(stderrors.New("EBUSY"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when Mkdir fails", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeDir, Mode: 0o750}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			targetMock.EXPECT().
				Mkdir(mock.Anything, path, fs.FileMode(0o750), 0, 0).
				Return(stderrors.New("ENOSPC"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when source Hash fails", func() {
			src := domain.FileNode{Kind: domain.NodeFile, Mode: 0o644, Size: 1}

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			sourceMock.EXPECT().Hash(mock.Anything, path).
				Return([32]byte{}, stderrors.New("io error"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when target Hash fails", func() {
			src := domain.FileNode{Kind: domain.NodeFile, Mode: 0o644, Size: 1}

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			sourceMock.EXPECT().Hash(mock.Anything, path).Return([32]byte{0xAA}, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).
				Return([32]byte{}, stderrors.New("io error"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when source Open fails", func() {
			src := domain.FileNode{Kind: domain.NodeFile, Mode: 0o644, Size: 1}

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			sourceMock.EXPECT().Open(mock.Anything, path).
				Return(nil, stderrors.New("EACCES"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when Chmod fails during metadata sync", func() {
			src := domain.FileNode{Kind: domain.NodeFile, Mode: 0o644, Size: 1}
			tgt := src
			tgt.Mode = 0o600

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0xAB}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chmod(mock.Anything, path, fs.FileMode(0o644)).
				Return(stderrors.New("EPERM"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when Chown fails during metadata sync", func() {
			src := domain.FileNode{Kind: domain.NodeFile, Mode: 0o644, UID: 1000, Size: 1}
			tgt := src
			tgt.UID = 2000

			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).Return(src, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).Return(tgt, nil)

			hash := [32]byte{0xCD}

			sourceMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Hash(mock.Anything, path).Return(hash, nil)
			targetMock.EXPECT().Chown(mock.Anything, path, 1000, 0).
				Return(stderrors.New("EPERM"))

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})

		It("wraps ErrSyncPath when source kind is unsupported", func() {
			matcherMock.EXPECT().Matches(path).Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeKind("weird")}, nil)
			targetMock.EXPECT().Stat(mock.Anything, path).
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)

			err := uc.Execute(ctx, path)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrSyncPath)).To(BeTrue())
		})
	})
})
