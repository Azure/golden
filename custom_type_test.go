package golden

import (
	"github.com/prashantv/gostub"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"testing"
)

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
			name = "sub${sub_block.key}"
			dynamic "sub_block" {
				for_each = ["2"]
				content {
					name = "sub${sub_block.key}"
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
			name = "sub${sub_block.key}"
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
