package golden

import (
	"github.com/prashantv/gostub"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
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

var _ Block = &SelfRefRootBlock{}

type SelfRefRootBlock struct {
	*BaseBlock
	WorkflowName string          `hcl:"name"`
	SubBlocks    []*SelfRefBlock `hcl:"sub_block,block"`
}

func (s *SelfRefRootBlock) Type() string {
	return ""
}

func (s *SelfRefRootBlock) BlockType() string {
	return "self_ref"
}

func (s *SelfRefRootBlock) AddressLength() int {
	return 2
}

func (s *SelfRefRootBlock) CanExecutePrePlan() bool {
	return false
}

type SelfRefBlock struct {
	NameString string `hcl:"name"`

	SubBlocks []*SelfRefBlock `hcl:"sub_block,block"`
}

func (s *SelfRefBlock) customCtyType(depth int) cty.Type {
	if depth == 0 {
		return cty.Object(map[string]cty.Type{
			"name": cty.String,
		})
	}
	return cty.Object(map[string]cty.Type{
		"name":      cty.String,
		"sub_block": cty.List(s.customCtyType(depth - 1)),
	})
}

func Test_SelfRefBlock(t *testing.T) {
	code := `self_ref this {
	name = "test"
    sub_block {
		name = "sub1"
		sub_block {
			name = "sub2"
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
	require.NoError(t, config.RunPlan())
	rootBlocks := Blocks[*SelfRefRootBlock](config)
	assert.Len(t, rootBlocks, 1)
	selfRefBlock := rootBlocks[0]
	assert.Equal(t, "test", selfRefBlock.WorkflowName)
	assert.Len(t, selfRefBlock.SubBlocks, 1)
	subBlock := selfRefBlock.SubBlocks[0]
	assert.Equal(t, "sub1", subBlock.NameString)
	assert.Len(t, subBlock.SubBlocks, 1)
	subSubBlock := subBlock.SubBlocks[0]
	assert.Equal(t, "sub2", subSubBlock.NameString)
}

func Test_MultipleSelfRefBlock(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{
			name: "self_ref with static blocks",
			code: `self_ref this {
	name = "test"
    sub_block {
		name = "sub1"
		sub_block {
			name = "sub2"
		}
	}
}

self_ref that {
	name = "test2"
   sub_block {
		name = "sub1"
	}
}
`,
		},
		{
			name: "self_ref with dynamic blocks",
			code: `self_ref this {
	name = "test"
    dynamic "sub_block" {
		for_each = ["1"]
		content {
			name = "sub${sub_block.value}"
			dynamic "sub_block" {
				for_each = ["2"]
				content {
					name = "sub${sub_block.value}"
				}
			}
		}
	}
}

self_ref that {
	name = "test2"
    dynamic "sub_block" {
		for_each = ["1"]
		content {
			name = "sub${sub_block.value}"
		}
	}
}
`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code := c.code
			mockFs := afero.NewMemMapFs()
			stub := gostub.Stub(&testFsFactory, func() afero.Fs {
				return mockFs
			})
			defer stub.Reset()
			_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

			config, err := BuildDummyConfig("", "", nil, nil)
			require.NoError(t, err)
			require.NoError(t, config.RunPlan())
			rootBlocks := Blocks[*SelfRefRootBlock](config)
			assert.Len(t, rootBlocks, 2)
			selfRefBlock1, selfRefBlock2 := rootBlocks[0], rootBlocks[1]
			if selfRefBlock1.WorkflowName == "test2" {
				selfRefBlock1, selfRefBlock2 = rootBlocks[1], rootBlocks[0]
			}
			assert.Equal(t, "test", selfRefBlock1.WorkflowName)
			assert.Len(t, selfRefBlock1.SubBlocks, 1)
			subBlock := selfRefBlock1.SubBlocks[0]
			assert.Equal(t, "sub1", subBlock.NameString)
			assert.Len(t, subBlock.SubBlocks, 1)
			assert.Equal(t, "sub2", subBlock.SubBlocks[0].NameString)

			assert.Equal(t, "test2", selfRefBlock2.WorkflowName)
			assert.Len(t, selfRefBlock2.SubBlocks, 1)
			subBlock = selfRefBlock2.SubBlocks[0]
			assert.Equal(t, "sub1", subBlock.NameString)
		})
	}
}
