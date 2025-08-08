package version

import (
	"fmt"
	"runtime"
)

// These variables are populated by the Go linker during the build process.
var (
	Version   = "dev"     // Overridden by the Git tag or dev version string
	Commit    = "none"    // Overridden by the Git commit hash
	Branch    = "unknown" // Overridden by the Git branch name
	BuildDate = "unknown" // Overridden by the build timestamp
)

// Info holds all the versioning information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Branch    string `json:"branch"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

// GetInfo returns a struct populated with the version information.
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Branch:    Branch,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted string of the version information.
func (i Info) String() string {
	return fmt.Sprintf(
		"Version:\t%s\nCommit:\t\t%s\nBranch:\t\t%s\nBuild Date:\t%s\nGo Version:\t%s\nCompiler:\t%s\nPlatform:\t%s",
		i.Version, i.Commit, i.Branch, i.BuildDate, i.GoVersion, i.Compiler, i.Platform,
	)
}