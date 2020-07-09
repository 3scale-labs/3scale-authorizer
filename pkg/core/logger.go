package core

type Logger interface {
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// NoOpLogger logs nothing
type NoOpLogger struct{}

func (l *NoOpLogger) Infof(s string, i ...interface{}) {}

func (l *NoOpLogger) Errorf(s string, i ...interface{}) {}

func (l *NoOpLogger) Debugf(s string, i ...interface{}) {}
