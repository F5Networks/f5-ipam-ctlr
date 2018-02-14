package orchestration_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestOrchestration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orchestration Suite")
}
