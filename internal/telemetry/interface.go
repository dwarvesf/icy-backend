package telemetry

type ITelemetry interface {
	IndexBtcTransaction() error
}
