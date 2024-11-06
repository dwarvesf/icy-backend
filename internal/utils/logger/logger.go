package logger

import (
	"go.uber.org/zap"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
)

type Logger struct {
	wrappedLogger *zap.Logger
}

func New(env environments.Environment) *Logger {
	var cfg zap.Config

	switch env {
	case environments.Development:
		cfg = newDevelopmentLoggerConfig()
	case environments.Test:
		cfg = newTestLoggerConfig()
	case environments.Staging:
		cfg = newStagingLoggerConfig()
	case environments.Production:
		cfg = newProductionLoggerConfig()
	default:
		cfg = newProductionLoggerConfig()
	}

	zapLogger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	return &Logger{
		wrappedLogger: zapLogger,
	}
}

func (l *Logger) Debug(msg string, inputFields ...map[string]string) {
	fields := []zap.Field{}

	if len(inputFields) > 0 {
		fields = transformStrMapToFields(inputFields[0])
	}

	l.wrappedLogger.Debug(msg, fields...)
}

func (l *Logger) Error(msg string, inputFields ...map[string]string) {
	fields := []zap.Field{}

	if len(inputFields) > 0 {
		fields = transformStrMapToFields(inputFields[0])
	}

	l.wrappedLogger.Error(msg, fields...)
}

func (l *Logger) Fatal(msg string, inputFields ...map[string]string) {
	fields := []zap.Field{}

	if len(inputFields) > 0 {
		fields = transformStrMapToFields(inputFields[0])
	}

	l.wrappedLogger.Fatal(msg, fields...)
}

func (l *Logger) Info(msg string, inputFields ...map[string]string) {
	fields := []zap.Field{}

	if len(inputFields) > 0 {
		fields = transformStrMapToFields(inputFields[0])
	}

	l.wrappedLogger.Info(msg, fields...)
}

func transformStrMapToFields(strMap map[string]string) []zap.Field {
	fields := []zap.Field{}
	for k, v := range strMap {
		fields = append(fields, zap.String(k, v))
	}

	return fields
}
