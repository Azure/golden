package golden

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIgnoreUnknownVariablesTrue(t *testing.T) {
	cliFlags := []CliFlagAssignedVariables{
		NewCliFlagAssignedVariable("unknown_variable", "value"),
	}
	config := NewBasicConfigFromArgs(NewBaseConfigArgs{
		IgnoreUnknownVariables:   true,
		CliFlagAssignedVariables: cliFlags,
	})
	variables, err := config.readCliAssignedVariables()
	require.NoError(t, err)
	require.Empty(t, variables)
}

func TestIgnoreUnknownVariablesFalse(t *testing.T) {
	cliFlags := []CliFlagAssignedVariables{
		NewCliFlagAssignedVariable("unknown_variable", "value"),
	}
	config := NewBasicConfigFromArgs(NewBaseConfigArgs{
		IgnoreUnknownVariables:   false,
		CliFlagAssignedVariables: cliFlags,
	})
	_, err := config.readCliAssignedVariables()
	assert.Error(t, err)
}
