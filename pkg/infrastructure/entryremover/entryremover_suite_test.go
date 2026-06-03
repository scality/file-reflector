package entryremover_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEntryRemover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EntryRemover Suite")
}
