package logger

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("Logger Environment", func() {
	Describe("#newProductionLoggerConfig", func() {
		It("should return the correct production logger configuration", func() {
			cfg := newProductionLoggerConfig()

			Expect(cfg.Level.Level()).To(Equal(zap.InfoLevel))
			Expect(cfg.Development).To(BeFalse())
			Expect(cfg.DisableCaller).To(BeFalse())
			Expect(cfg.DisableStacktrace).To(BeFalse())
			Expect(cfg.Encoding).To(Equal("json"))
			Expect(cfg.OutputPaths).To(Equal([]string{"stdout"}))
			Expect(cfg.ErrorOutputPaths).To(Equal([]string{"stderr"}))
		})
	})

	Describe("#newStagingLoggerConfig", func() {
		It("should return the correct staging logger configuration", func() {
			cfg := newStagingLoggerConfig()

			Expect(cfg.Level.Level()).To(Equal(zap.InfoLevel))
			Expect(cfg.Development).To(BeFalse())
			Expect(cfg.DisableCaller).To(BeTrue())
			Expect(cfg.DisableStacktrace).To(BeTrue())
			Expect(cfg.Encoding).To(Equal("json"))
			Expect(cfg.OutputPaths).To(Equal([]string{"stdout"}))
			Expect(cfg.ErrorOutputPaths).To(Equal([]string{"stderr"}))
		})
	})

	Describe("#newDevelopmentLoggerConfig", func() {
		It("should return the correct development logger configuration", func() {
			cfg := newDevelopmentLoggerConfig()

			Expect(cfg.Level.Level()).To(Equal(zap.DebugLevel))
			Expect(cfg.Development).To(BeTrue())
			Expect(cfg.DisableCaller).To(BeTrue())
			Expect(cfg.DisableStacktrace).To(BeTrue())
			Expect(cfg.Encoding).To(Equal("console"))
			Expect(cfg.OutputPaths).To(Equal([]string{"stdout"}))
			Expect(cfg.ErrorOutputPaths).To(Equal([]string{"stderr"}))
		})
	})

	Describe("#newTestLoggerConfig", func() {
		It("should return the correct test logger configuration", func() {
			cfg := newTestLoggerConfig()

			Expect(cfg.Level.Level()).To(Equal(zap.InfoLevel))
			Expect(cfg.Development).To(BeFalse())
			Expect(cfg.DisableCaller).To(BeFalse())
			Expect(cfg.DisableStacktrace).To(BeFalse())
			Expect(cfg.Encoding).To(Equal("json"))
			Expect(cfg.OutputPaths).To(BeEmpty())
			Expect(cfg.ErrorOutputPaths).To(BeEmpty())
		})
	})
})
