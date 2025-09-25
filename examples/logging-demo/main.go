package main

import (
	"errors"
	"time"

	"github.com/mattsolo1/grove-core/logging"
	"github.com/sirupsen/logrus"
)

func main() {
	// Create loggers for different components
	appLog := logging.NewLogger("demo-app")
	dbLog := logging.NewLogger("database")
	apiLog := logging.NewLogger("api")

	// Basic logging
	appLog.Info("Application starting...")

	// Structured logging with fields
	apiLog.WithFields(logrus.Fields{
		"port":    8080,
		"host":    "localhost",
		"version": "1.0.0",
	}).Info("API server configured")

	// Simulating database connection
	dbLog.Debug("Attempting database connection")
	time.Sleep(100 * time.Millisecond)
	dbLog.WithField("connection_pool_size", 10).Info("Database connected")

	// Warning example
	apiLog.WithFields(logrus.Fields{
		"endpoint":      "/api/v1/users",
		"response_time": 1500,
	}).Warn("Slow API response detected")

	// Error example with WithError
	err := errors.New("connection timeout")
	dbLog.WithError(err).WithFields(logrus.Fields{
		"retry_count": 3,
		"timeout_ms":  5000,
	}).Error("Failed to execute query")

	// Different log levels
	appLog.Debug("This is debug information")
	appLog.Info("This is an info message")
	appLog.Warn("This is a warning")
	appLog.Error("This is an error")

	// Example of progressive enhancement with fields
	log := appLog.WithField("request_id", "abc-123")
	log.Info("Processing request")
	
	log = log.WithField("user_id", 456)
	log.Info("User authenticated")
	
	log = log.WithField("action", "create_post")
	log.Info("Performing action")

	appLog.Info("Application shutdown complete")
}