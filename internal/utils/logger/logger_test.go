package logger

import (
	"bytes"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
)

type customWriteHook struct {
	called bool
}

func (h *customWriteHook) OnWrite(_ *zapcore.CheckedEntry, _ []zapcore.Field) {
	h.called = true
}

var _ = Describe("Logger", func() {
	var logger *Logger

	Describe("#New", func() {
		It("should create a new logger with production config when environment is production", func() {
			logger = New(environments.Production)
			Expect(logger).NotTo(BeNil())
			Expect(logger.wrappedLogger).NotTo(BeNil())
		})

		It("should create a new logger with development config when environment is development", func() {
			logger = New(environments.Development)
			Expect(logger).NotTo(BeNil())
			Expect(logger.wrappedLogger).NotTo(BeNil())
		})

		It("should create a new logger with staging config when environment is staging", func() {
			logger = New(environments.Staging)
			Expect(logger).NotTo(BeNil())
			Expect(logger.wrappedLogger).NotTo(BeNil())
		})

		It("should create a new logger with test config when environment is test", func() {
			logger = New(environments.Test)
			Expect(logger).NotTo(BeNil())
			Expect(logger.wrappedLogger).NotTo(BeNil())
		})

		It("should create a new logger with production config when environment is unknown", func() {
			unknownEnv := environments.Environment("unknown")
			logger = New(unknownEnv)
			Expect(logger).NotTo(BeNil())
			Expect(logger.wrappedLogger).NotTo(BeNil())

			// Verify that the logger is configured with production settings
			zapLogger := logger.wrappedLogger.WithOptions(zap.AddCaller())
			core := zapLogger.Core()
			Expect(core.Enabled(zapcore.InfoLevel)).To(BeTrue())
			Expect(core.Enabled(zapcore.DebugLevel)).To(BeFalse())
		})
	})

	Describe("#Debug", func() {
		BeforeEach(func() {
			logger = New(environments.Test)
		})

		It("should log debug messages", func() {
			Expect(func() {
				logger.Debug("debug message", map[string]string{"key": "value"})
			}).NotTo(Panic())
		})
	})

	Describe("#Error", func() {
		BeforeEach(func() {
			logger = New(environments.Test)
		})

		It("should log error messages", func() {
			Expect(func() {
				logger.Error("error message", map[string]string{"key": "value"})
			}).NotTo(Panic())
		})
	})

	Describe("#Info", func() {
		BeforeEach(func() {
			logger = New(environments.Test)
		})

		It("should log info messages", func() {
			Expect(func() {
				logger.Info("info message", map[string]string{"key": "value"})
			}).NotTo(Panic())
		})
	})

	Describe("#Fatal", func() {
		BeforeEach(func() {
			logger = New(environments.Test)
		})

		It("should log fatal messages", func() {
			hook := &customWriteHook{}
			originalLogger := logger.wrappedLogger
			defer func() { logger.wrappedLogger = originalLogger }()

			testLogger := zap.New(
				zapcore.NewCore(
					zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
					zapcore.AddSync(&bytes.Buffer{}),
					zap.FatalLevel,
				),
				zap.WithFatalHook(hook),
			)
			logger.wrappedLogger = testLogger

			logger.Fatal("fatal message", map[string]string{"key": "value"})
			Expect(hook.called).To(BeTrue())
		})
	})

	Describe("#transformStrMapToFields", func() {
		It("should transform a string map to zap fields", func() {
			inputMap := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			fields := transformStrMapToFields(inputMap)

			// sort fields by key
			sort.Slice(fields, func(i, j int) bool {
				return fields[i].Key < fields[j].Key
			})

			Expect(fields).To(HaveLen(2))
			Expect(fields[0]).To(Equal(zap.String("key1", "value1")))
			Expect(fields[1]).To(Equal(zap.String("key2", "value2")))
		})

		It("should return an empty slice for an empty input map", func() {
			inputMap := map[string]string{}
			fields := transformStrMapToFields(inputMap)
			Expect(fields).To(BeEmpty())
		})
	})
})
