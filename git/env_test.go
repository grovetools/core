package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"

	// TODO: Update when testutil is available in grove-core
	// "github.com/mattsolo1/grove-core/testutil"
)

// TODO: Re-enable this test when testutil package is available in grove-core
/*
func TestGetEnvironmentVars(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitGitRepo(t, tmpDir)

	// Set git config
	testutil.RunGitCommand(t, tmpDir, "config", "user.name", "Test User")
	testutil.RunGitCommand(t, tmpDir, "config", "user.email", "test@example.com")

	vars, err := GetEnvironmentVars(tmpDir)
	require.NoError(t, err)

	assert.NotEmpty(t, vars.Repo)
	assert.NotEmpty(t, vars.Branch)
	assert.NotEmpty(t, vars.Commit)
	assert.Equal(t, "Test User", vars.Author)
	assert.Equal(t, "test@example.com", vars.AuthorEmail)
	assert.False(t, vars.IsDirty)
}
*/

func TestEnvironmentVars_ToMap(t *testing.T) {
	vars := &EnvironmentVars{
		Repo:        "myproject",
		Branch:      "feature-123",
		Commit:      "abcdef1234567890",
		CommitShort: "abcdef1",
		Author:      "John Doe",
		AuthorEmail: "john@example.com",
		IsDirty:     true,
	}

	envMap := vars.ToMap()

	assert.Equal(t, "myproject", envMap["GROVE_REPO"])
	assert.Equal(t, "feature-123", envMap["GROVE_BRANCH"])
	assert.Equal(t, "abcdef1234567890", envMap["GROVE_COMMIT"])
	assert.Equal(t, "abcdef1", envMap["GROVE_COMMIT_SHORT"])
	assert.Equal(t, "John Doe", envMap["GROVE_AUTHOR"])
	assert.Equal(t, "true", envMap["GROVE_IS_DIRTY"])

	// Check aliases
	assert.Equal(t, "myproject", envMap["REPO"])
	assert.Equal(t, "feature-123", envMap["BRANCH"])
	assert.Equal(t, "abcdef1", envMap["COMMIT"])
}