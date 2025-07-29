package cli

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// LoggerOption represents a function that configures a logger
type LoggerOption func(*logrus.Logger)

// WithOutput sets the logger output
func WithOutput(w io.Writer) LoggerOption {
	return func(l *logrus.Logger) {
		l.SetOutput(w)
	}
}

// WithLevel sets the log level
func WithLevel(level logrus.Level) LoggerOption {
	return func(l *logrus.Logger) {
		l.SetLevel(level)
	}
}

// WithFormatter sets the log formatter
func WithFormatter(formatter logrus.Formatter) LoggerOption {
	return func(l *logrus.Logger) {
		l.SetFormatter(formatter)
	}
}

// NewLogger creates a new logger with options
func NewLogger(opts ...LoggerOption) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	
	for _, opt := range opts {
		opt(logger)
	}
	
	return logger
}