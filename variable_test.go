package golden

import (
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
