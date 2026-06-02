package usecase_test

import (
	"context"
	stderrors "errors"
	"io"
	"log/slog"
	"slices"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"
	"github.com/stretchr/testify/mock"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service/mocks"
	"github.com/scality/file-reflector/pkg/usecase"
)

// recordingSyncer is a hand-rolled pathSyncer that captures the order
// in which Execute is invoked. Set err to fail on every call, or failOn
// to fail only on a specific relPath.
type recordingSyncer struct {
	mu     sync.Mutex
	calls  []string
	err    error
	failOn string
}

func (r *recordingSyncer) Execute(_ context.Context, relPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, relPath)

	if r.err != nil {
		return r.err
	}

	if r.failOn != "" && relPath == r.failOn {
		return stderrors.New("recorder failure for " + relPath)
	}

	return nil
}

func (r *recordingSyncer) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.calls...)
}

var _ = Describe("InitialSync", func() {
	var (
		logger      *slog.Logger
		sourceMock  *mocks.MockSourceReader
		targetMock  *mocks.MockTargetWriter
		matcherMock *mocks.MockMatcher
		syncer      *recordingSyncer
		uc          *usecase.InitialSync
		ctx         context.Context

		dir  domain.FileNode
		file domain.FileNode
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		sourceMock = mocks.NewMockSourceReader(GinkgoT())
		targetMock = mocks.NewMockTargetWriter(GinkgoT())
		matcherMock = mocks.NewMockMatcher(GinkgoT())
		syncer = &recordingSyncer{}
		uc = usecase.NewInitialSync(logger, sourceMock, targetMock, matcherMock, syncer)
		ctx = context.Background()

		dir = domain.FileNode{Kind: domain.NodeDir}
		file = domain.FileNode{Kind: domain.NodeFile}
	})

	Describe("with an empty source and target", func() {
		It("only reconciles the root", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)

			Expect(uc.Execute(ctx)).To(Succeed())
			Expect(syncer.seen()).To(Equal([]string{""}))
		})
	})

	Describe("source walk", func() {
		It("visits entries top-down", func() {
			// Source tree: "", "foo" (dir), "foo/bar" (file), "baz" (file)
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"foo", "baz"}, nil)
			matcherMock.EXPECT().Matches("foo").Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, "foo").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "foo").Return([]string{"bar"}, nil)
			matcherMock.EXPECT().Matches("foo/bar").Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, "foo/bar").Return(file, nil)
			matcherMock.EXPECT().Matches("baz").Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, "baz").Return(file, nil)
			// Empty target.
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)

			Expect(uc.Execute(ctx)).To(Succeed())

			seen := syncer.seen()
			Expect(seen).To(HaveLen(4))
			Expect(seen[0]).To(Equal(""))
			Expect(seen[1]).To(Equal("foo"))
			// foo's children come before baz (depth-first per ReadDir order).
			Expect(seen[2]).To(Equal("foo/bar"))
			Expect(seen[3]).To(Equal("baz"))
		})

		It("skips ignored directories entirely (no SyncPath, no recursion)", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"cache"}, nil)
			matcherMock.EXPECT().Matches("cache").Return(true)
			// No Stat/ReadDir on "cache", no SyncPath either.
			// Empty target.
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)

			Expect(uc.Execute(ctx)).To(Succeed())
			Expect(syncer.seen()).To(Equal([]string{""}))
		})
	})

	Describe("target cleanup", func() {
		It("invokes SyncPath on target entries absent from source", func() {
			// Source has only the root.
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			// Target has "extra" (file) and "stale" (dir with one file).
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"extra", "stale"}, nil)
			matcherMock.EXPECT().Matches("extra").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "extra").Return(file, nil)
			matcherMock.EXPECT().Matches("stale").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "stale").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "stale").Return([]string{"orphan"}, nil)
			matcherMock.EXPECT().Matches("stale/orphan").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "stale/orphan").Return(file, nil)

			Expect(uc.Execute(ctx)).To(Succeed())

			seen := syncer.seen()
			Expect(seen[0]).To(Equal(""))
			// Bottom-up: orphan before stale; extra and stale are sibling order.
			Expect(seen[1:]).To(ConsistOf("extra", "stale/orphan", "stale"))

			orphanIdx := slices.Index(seen, "stale/orphan")
			staleIdx := slices.Index(seen, "stale")
			Expect(orphanIdx).To(BeNumerically("<", staleIdx),
				"stale/orphan must be reconciled before its parent stale")
		})

		It("does not invoke SyncPath on target entries that exist in source", func() {
			// Source: "" and "keep" (file)
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"keep"}, nil)
			matcherMock.EXPECT().Matches("keep").Return(false)
			sourceMock.EXPECT().Stat(mock.Anything, "keep").Return(file, nil)
			// Target: "" and "keep" (file)
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"keep"}, nil)
			matcherMock.EXPECT().Matches("keep").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "keep").Return(file, nil)

			Expect(uc.Execute(ctx)).To(Succeed())
			// "keep" is reconciled once during source walk, not again during cleanup.
			Expect(syncer.seen()).To(Equal([]string{"", "keep"}))
		})

		It("skips recursion into ignored target directories", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			// Target has a "cache" dir; the matcher ignores it so we don't
			// touch it at all (no Stat/ReadDir on its descendants either).
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"cache"}, nil)
			matcherMock.EXPECT().Matches("cache").Return(true)

			Expect(uc.Execute(ctx)).To(Succeed())
			Expect(syncer.seen()).To(Equal([]string{""}))
		})

		It("does nothing when the target root is absent", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			targetMock.EXPECT().Stat(mock.Anything, "").
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)

			Expect(uc.Execute(ctx)).To(Succeed())
			Expect(syncer.seen()).To(Equal([]string{""}))
		})

		It("cleans up the whole target when source root is absent", func() {
			// Source root reports absent: walkSource records the root then
			// returns without recursing.
			sourceMock.EXPECT().Stat(mock.Anything, "").
				Return(domain.FileNode{Kind: domain.NodeAbsent}, nil)
			// Target has one stale file; cleanup should reconcile it.
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"stale"}, nil)
			matcherMock.EXPECT().Matches("stale").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "stale").Return(file, nil)

			Expect(uc.Execute(ctx)).To(Succeed())
			// SyncPath fires for "" (source walk) and "stale" (cleanup).
			Expect(syncer.seen()).To(Equal([]string{"", "stale"}))
		})
	})

	Describe("error propagation", func() {
		It("wraps ErrInitialSync when SyncPath fails during the source walk", func() {
			syncer.err = stderrors.New("disk on fire")
			// SyncPath fires for "" and returns the error.

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})

		It("wraps ErrInitialSync when source Stat fails mid-walk", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(domain.FileNode{}, stderrors.New("ENOENT"))

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})

		It("wraps ErrInitialSync when target Stat fails during cleanup", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			targetMock.EXPECT().Stat(mock.Anything, "").Return(domain.FileNode{}, stderrors.New("EIO"))

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})

		It("wraps ErrInitialSync when source ReadDir fails mid-walk", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").
				Return(nil, stderrors.New("EACCES"))

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})

		It("wraps ErrInitialSync when target ReadDir fails during cleanup", func() {
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").
				Return(nil, stderrors.New("EACCES"))

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})

		It("wraps ErrInitialSync when SyncPath fails during target cleanup", func() {
			// Source has only the root; target has an orphan that triggers
			// SyncPath on the cleanup pass, which the recorder fails.
			sourceMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			sourceMock.EXPECT().ReadDir(mock.Anything, "").Return(nil, nil)
			targetMock.EXPECT().Stat(mock.Anything, "").Return(dir, nil)
			targetMock.EXPECT().ReadDir(mock.Anything, "").Return([]string{"orphan"}, nil)
			matcherMock.EXPECT().Matches("orphan").Return(false)
			targetMock.EXPECT().Stat(mock.Anything, "orphan").Return(file, nil)

			// Recorder lets the source-walk SyncPath("") succeed, then fails
			// on the cleanup SyncPath("orphan").
			syncer.failOn = "orphan"

			err := uc.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, usecase.ErrInitialSync)).To(BeTrue())
		})
	})
})
