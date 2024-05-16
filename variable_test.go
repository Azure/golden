package golden

import (
	"context"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
	"testing"
)

type variableSuite struct {
	suite.Suite
	*testBase
}

func TestVariableSuite(t *testing.T) {
	suite.Run(t, new(variableSuite))
}

func (s *variableSuite) SetupTest() {
	s.testBase = newTestBase()
}

func (s *variableSuite) TearDownTest() {
	s.teardown()
}

func (s *variableSuite) TestVariableBlockWithoutTypeShouldHasNilVariableType() {
	code := `variable "test" {
}`
	file, diag := hclsyntax.ParseConfig([]byte(code), "test.hcl", hcl.InitialPos)
	require.False(s.T(), diag.HasErrors())
	sut := &VariableBlock{
		BaseBlock: &BaseBlock{
			hb: &HclBlock{
				Block: file.Body.(*hclsyntax.Body).Blocks[0],
			},
		},
	}
	err := sut.parseVariableType()
	s.NoError(err)
	s.Nil(sut.VariableType)
}

func (s *variableSuite) TestVariableBlockWithTypeShouldParseVariableType() {
	code := `variable "test" {
  type = string
}`
	file, diag := hclsyntax.ParseConfig([]byte(code), "test.hcl", hcl.InitialPos)
	require.False(s.T(), diag.HasErrors())
	sut := &VariableBlock{
		BaseBlock: &BaseBlock{
			hb: &HclBlock{
				Block: file.Body.(*hclsyntax.Body).Blocks[0],
			},
		},
	}
	err := sut.parseVariableType()
	s.NoError(err)
	s.Equal(cty.String, *sut.VariableType)
}

func (s *variableSuite) TestReadValueFromEnv() {
	cases := []struct {
		desc        string
		valueString string
		expected    cty.Value
	}{
		{
			desc:        "string value",
			valueString: `hello`,
			expected:    cty.StringVal("hello"),
		},
		{
			desc:        "bool",
			valueString: "true",
			expected:    cty.True,
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.T().Setenv(fmt.Sprintf("FT_VAR_test"), c.valueString)
			config, err := NewDummyConfig(".", context.TODO(), nil)
			require.NoError(s.T(), err)
			sut := &VariableBlock{
				BaseBlock: &BaseBlock{
					c: config,
				},
			}
			sut.name = "test"
			read := sut.readValueFromEnv()
			s.NoError(read.Error)
			s.Equal(c.expected, *read.Value)
		})
	}
}

func (s *variableSuite) TestReadDefaultValue() {
	cases := []struct {
		desc                string
		variableDefiniation string
		expected            VariableValueRead
	}{
		{
			desc: "string default value",
			variableDefiniation: `variable "test" {
  default = "hello"
}`,
			expected: NewVariableValueRead("test", p(cty.StringVal("hello")), nil),
		},
		{
			desc: "no default value",
			variableDefiniation: `variable "test" {
}`,
			expected: NoValue,
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			rfile, diag := hclsyntax.ParseConfig([]byte(c.variableDefiniation), "test.hcl", hcl.InitialPos)
			require.False(s.T(), diag.HasErrors())
			wfile, diag := hclwrite.ParseConfig([]byte(c.variableDefiniation), "test.hcl", hcl.InitialPos)
			require.False(s.T(), diag.HasErrors())
			sut := &VariableBlock{
				BaseBlock: &BaseBlock{
					name: "test",
					hb:   NewHclBlock(rfile.Body.(*hclsyntax.Body).Blocks[0], wfile.Body().Blocks()[0], nil),
				},
			}
			read := sut.readDefaultValue()
			s.Equal(c.expected, read)
		})
	}
}

func (s *variableSuite) TestReadValueFromEnv_EmptyEnvShouldReturnNilCtyValue() {
	config, err := NewDummyConfig(".", context.TODO(), nil)
	require.NoError(s.T(), err)
	sut := &VariableBlock{
		BaseBlock: &BaseBlock{
			c: config,
		},
	}
	sut.name = "test"
	read := sut.readValueFromEnv()
	s.NoError(read.Error)
	s.Nil(read.Value)
}

func (s *variableSuite) TestReadVariableValue_ReadDefaultIfNotSet() {
	cases := []struct {
		desc     string
		cliFlags []cliFlagAssignedVariables
		files    map[string]string
		expected VariableValueRead
	}{
		{
			desc:     "no value set",
			expected: NewVariableValueRead("string_value", p(cty.StringVal("world")), nil),
		},
		{
			desc: "cliFlagAssignedVariableFile-hcl",
			cliFlags: []cliFlagAssignedVariables{
				cliFlagAssignedVariableFile{
					varFileName: "/test.tfvars",
				},
			},
			files: map[string]string{
				"/test.tfvars": `string_value = "hello"`,
			},
			expected: NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
		},
	}

	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": `variable "string_value" {
  default = "world"
}`,
			})
			config, err := BuildDummyConfig("/", "", nil)
			require.NoError(s.T(), err)
			cfg := config.(*DummyConfig).BaseConfig
			cfg.cliFlagAssignedVariables = c.cliFlags
			variableBlocks := Blocks[*VariableBlock](cfg)
			vb := variableBlocks[0]
			read := vb.readValue()
			s.Equal(c.expected, read)
		})
	}
}

func p[T any](input T) *T {
	return &input
}
