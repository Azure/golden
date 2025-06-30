package golden

import (
	"github.com/prashantv/gostub"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
)

func TestNewHclBlock(t *testing.T) {
	// Define your HCL configuration as a string
	hclConfig := `
    block "test" {
        attr1 = "value1"
        attr2 = "value2"
        nested {
			attr3 = "value3"
        }
    }
    `

	// Parse the HCL configuration using hclsyntax.ParseConfig
	syntaxFile, diag := hclsyntax.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
	if diag.HasErrors() {
		t.Fatalf("Failed to parse HCL: %s", diag.Error())
	}

	// Parse the HCL configuration using hclwrite.ParseConfig
	writeFile, diag := hclwrite.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
	if diag.HasErrors() {
		t.Fatalf("Failed to parse HCL: %s", diag.Error())
	}

	// Get the first block from the parsed HCL configuration
	syntaxBlock := syntaxFile.Body.(*hclsyntax.Body).Blocks[0]
	writeBlock := writeFile.Body().Blocks()[0]

	// Call NewHclBlock
	hclBlock := NewHclBlock(syntaxBlock, writeBlock, nil)

	// Verify that the attributes were loaded correctly
	assert.Equal(t, 2, len(hclBlock.Attributes()))
	assert.NotNil(t, hclBlock.Attributes()["attr1"])
	assert.NotNil(t, hclBlock.Attributes()["attr2"])

	// Verify that the nested blocks were loaded correctly
	assert.Equal(t, 1, len(hclBlock.NestedBlocks()))
	nb := hclBlock.NestedBlocks()[0]
	assert.Equal(t, "nested", nb.Type)
	assert.Equal(t, 1, len(nb.Attributes()))
	assert.NotNil(t, nb.Attributes()["attr3"])
}

func TestDynamicBlock_iteratorKey(t *testing.T) {
	code := `resource "dummy" this {
	dynamic "nested_block" {
		for_each = { id = 1 }
		content {
			id = nested_block.value
			name = "test-${nested_block.key}"
		}
    }
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	_, err = RunDummyPlan(config)
	require.NoError(t, err)
	rootBlock := Blocks[*DummyResource](config)[0]
	assert.Len(t, rootBlock.NestedBlocks, 1)
	assert.Equal(t, 1, rootBlock.NestedBlocks[0].Id)
	assert.Equal(t, "test-id", rootBlock.NestedBlocks[0].Name)
}
