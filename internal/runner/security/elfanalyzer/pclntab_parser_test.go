//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPclntabParser_NoPclntab(t *testing.T) {
	// This test verifies the error message when .gopclntab is missing
	assert.Equal(t, "no .gopclntab section found", ErrNoPclntab.Error())
}

func TestPclntabParser_FindFunction(t *testing.T) {
	result := &PclntabResult{}

	// Function not found when result is empty
	_, found := result.FindFunction("main.main")
	assert.False(t, found)
}

func TestPclntabResult_FindFunction(t *testing.T) {
	result := &PclntabResult{
		Functions: map[string]PclntabFunc{
			"main.main": {Name: "main.main", Entry: 0x401000, End: 0x401100},
		},
	}

	fn, found := result.FindFunction("main.main")
	assert.True(t, found)
	assert.Equal(t, "main.main", fn.Name)
	assert.Equal(t, uint64(0x401000), fn.Entry)
	assert.Equal(t, uint64(0x401100), fn.End)

	_, found = result.FindFunction("nonexistent")
	assert.False(t, found)
}
