package metadatareader_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMetadataReader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MetadataReader Suite")
}
