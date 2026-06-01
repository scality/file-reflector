package contentreader_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentReader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContentReader Suite")
}
