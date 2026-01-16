package logging_test

import (
	"github.com/grovetools/core/logging"
	"github.com/sirupsen/logrus"
)

func ExampleNewLogger() {
	// Create a logger for your component
	log := logging.NewLogger("my-component")
	
	// Use it for various log levels
	log.Debug("Debug information")
	log.Info("Starting process")
	log.Warn("Resource usage high")
	log.Error("Connection failed")
	
	// Add structured fields
	log.WithFields(logrus.Fields{
		"user_id": 123,
		"action":  "login",
	}).Info("User logged in")
	
	// Use WithField for single fields
	log.WithField("file", "/path/to/file.txt").Info("Processing file")
	
	// Use WithError for errors
	// err := someFunction()
	// log.WithError(err).Error("Operation failed")
}

func ExampleNewLogger_configuration() {
	// Configuration via grove.yml:
	//
	// logging:
	//   level: debug              # Set log level
	//   report_caller: true       # Include file/line info
	//   file:
	//     enabled: true
	//     path: /var/log/grove/app.log
	//   format:
	//     preset: json           # Use JSON output format
	
	// Or via environment variables:
	// GROVE_LOG_LEVEL=debug
	// GROVE_LOG_CALLER=true
	
	log := logging.NewLogger("configured-app")
	log.Info("This will respect the configuration")
}

func ExampleNewLogger_multipleComponents() {
	// Different components can have their own loggers
	// but they share the same configuration
	
	dbLog := logging.NewLogger("database")
	apiLog := logging.NewLogger("api-server")
	authLog := logging.NewLogger("auth")
	
	// Each log entry will be tagged with its component
	dbLog.Info("Connected to database")
	apiLog.Info("Server started on port 8080")
	authLog.Warn("Invalid login attempt")
	
	// Output will show:
	// [INFO] [database] Connected to database
	// [INFO] [api-server] Server started on port 8080
	// [WARN] [auth] Invalid login attempt
}