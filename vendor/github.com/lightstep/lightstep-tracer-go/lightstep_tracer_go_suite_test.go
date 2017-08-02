package lightstep_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLightstepTracerGo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LightstepTracerGo Suite")
}
