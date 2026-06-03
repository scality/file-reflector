package contenthasher_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentHasher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContentHasher Suite")
}
