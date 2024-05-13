package golden

import (
	"context"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
	err := sut.ParseVariableType()
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
	err := sut.ParseVariableType()
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
			actual, err := sut.ReadValueFromEnv()
			s.NoError(err)
			s.Equal(c.expected, *actual)
		})
	}
}

func (s *variableSuite) TestReadValueFromEnv_EmptyEnvShouldReturnNilCtyValue() {
	sut := &VariableBlock{
		BaseBlock: &BaseBlock{},
	}
	sut.name = "test"
	actual, err := sut.ReadValueFromEnv()
	s.NoError(err)
	s.Nil(actual)
}

func p[T any](input T) *T {
	return &input
}
