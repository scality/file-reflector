package contentwriter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentWriter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContentWriter Suite")
}
