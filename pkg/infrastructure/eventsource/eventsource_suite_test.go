package eventsource_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventSource Suite")
}
