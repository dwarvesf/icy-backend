package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Config", func() {
	Describe("#ApplicationConfig", func() {
		BeforeEach(func() {
			os.Unsetenv("LIVENESS_PROBE_PORT")
		})

	})
})
