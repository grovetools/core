This guide provides a step-by-step tutorial for building a basic command-line tool using the `grove-core` library. You will create a "hello world" application that demonstrates command creation, logging, flag handling, and configuration loading.

### Prerequisites

*   Go 1.21 or later installed.
*   A properly configured Go development environment.

## 1. Setup

First, create a new directory for your project and initialize a Go module.

```sh
mkdir my-grove-tool
cd my-grove-tool
go mod init my-grove-tool
```

Next, add `grove-core` as a dependency to your project:

```sh
go get github.com/mattsolo1/grove-core
```

This command downloads the library and adds it to your project's `go.mod` file.

## 2. Create a Basic Command

The `cli` package is the foundation for building commands. The `cli.NewStandardCommand` function creates a `cobra.Command` with standard flags like `--verbose`, `--json`, and `--config` already configured.

Create a `main.go` file with the following content:

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"hello",
		"A simple hello world tool using grove-core",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

This code sets up a root command but doesn't add any specific logic yet. You can build and run it to see the default help output.

```sh
go build .
./my-grove-tool --help
```

## 3. Add Command Logic

To make the command do something, you assign a function to its `Run` field. This function will be executed when the command is run.

Modify `main.go` to print a "Hello, World!" message.

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"hello",
		"A simple hello world tool using grove-core",
	)

	// Add the logic to the command's Run function
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello, World!")
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

Build and run the tool again:

```sh
go build .
./my-grove-tool
# Output: Hello, World!
```

## 4. Use the Logging Package

`grove-core` provides a centralized logging system built on `logrus`. All logs are written to `stderr` by default, separating them from program output on `stdout`.

Let's replace the `fmt.Println` with a structured logger.

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.comcom/mattsolo1/grove-core/cli"
	"github.comcom/mattsolo1/grove-core/logging"
	"github.comcom/spf13/cobra"
)

func main() {
	// Create a logger for our application component
	log := logging.NewLogger("hello-tool")

	rootCmd := cli.NewStandardCommand(
		"hello",
		"A simple hello world tool using grove-core",
	)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		log.Info("Hello, World!")
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

When you run this version, the output will be a formatted log message sent to `stderr`.

```sh
go build .
./my-grove-tool
# Output (to stderr):
# 2025-10-27 10:30:00 [INFO] [hello-tool] Hello, World!
```

## 5. Handle Standard Flags

The `NewStandardCommand` function automatically adds a `--verbose` (or `-v`) flag. You can use this flag to control the log level.

Let's modify the code to check for the `--verbose` flag and set the log level to `Debug` if it's present.

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	log := logging.NewLogger("hello-tool")

	rootCmd := cli.NewStandardCommand(
		"hello",
		"A simple hello world tool using grove-core",
	)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		// Check for the verbose flag
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			// Access the underlying logger to set the level
			log.Logger.SetLevel(logrus.DebugLevel)
			log.Debug("Verbose logging enabled")
		}

		log.Info("Hello, World!")
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

Now, running the command with the `-v` flag will produce additional debug output.

```sh
go build .
./my-grove-tool -v
# Output (to stderr):
# 2025-10-27 10:35:00 [DEBUG] [hello-tool] Verbose logging enabled
# 2025-10-27 10:35:00 [INFO] [hello-tool] Hello, World!
```

## 6. Use the Configuration System

`grove-core` provides a hierarchical system for loading `grove.yml` configuration files. Let's create a configuration file and load a custom setting.

First, create a `grove.yml` file in your project directory:

```yaml
# grove.yml
version: "1.0"

# Custom section for our tool
greeting:
  message: "Hello from configuration!"
```

Next, update `main.go` to load this file and use the custom message. This involves defining a struct to hold our custom configuration and using `config.UnmarshalExtension` to parse it.

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Define a struct for our custom configuration section
type GreetingConfig struct {
	Message string `yaml:"message"`
}

func main() {
	log := logging.NewLogger("hello-tool")

	rootCmd := cli.NewStandardCommand(
		"hello",
		"A simple hello world tool using grove-core",
	)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			log.Logger.SetLevel(logrus.DebugLevel)
			log.Debug("Verbose logging enabled")
		}

		// Load configuration from grove.yml
		cfg, err := config.LoadDefault()
		if err != nil {
			log.WithError(err).Fatal("Failed to load configuration")
		}

		// Unmarshal our custom "greeting" section
		var greetingCfg GreetingConfig
		if err := cfg.UnmarshalExtension("greeting", &greetingCfg); err != nil {
			log.WithError(err).Fatal("Failed to parse 'greeting' configuration")
		}

		// Use the message from the config file, or a default
		message := "Hello, World!"
		if greetingCfg.Message != "" {
			message = greetingCfg.Message
		}
		
		log.WithField("source", "grove.yml").Info(message)
	}

	if err := rootCmd.Execute(); err != nil {
		// Using a logger here is not ideal as it may not be initialized.
		// For application entrypoint errors, fmt is acceptable.
		fmt.Println(err)
		os.Exit(1)
	}
}
```

Now when you run the tool, it will print the message from your `grove.yml` file.

```sh
go build .
./my-grove-tool
# Output (to stderr):
# 2025-10-27 10:40:00 [INFO] [hello-tool] Hello from configuration! source=grove.yml
```