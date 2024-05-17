package golden

import (
	"context"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/prashantv/gostub"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
	"regexp"
	"strings"
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
	s.Nil(sut.variableType)
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
	s.Equal(cty.String, *sut.variableType)
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
			s.T().Setenv("FT_VAR_test", c.valueString)
			config, err := NewDummyConfig(".", context.TODO(), nil, nil)
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
	config, err := NewDummyConfig(".", context.TODO(), nil, nil)
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
		cliFlags []CliFlagAssignedVariables
		files    map[string]string
		expected VariableValueRead
	}{
		{
			desc:     "no value set",
			expected: NewVariableValueRead("string_value", p(cty.StringVal("world")), nil),
		},
		{
			desc: "CliFlagAssignedVariableFile-hcl",
			cliFlags: []CliFlagAssignedVariables{
				CliFlagAssignedVariableFile{
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
			config, err := BuildDummyConfig("/", "", c.cliFlags, nil)
			require.NoError(s.T(), err)
			cfg := config.(*DummyConfig).BaseConfig
			variableBlocks := Blocks[*VariableBlock](cfg)
			vb := variableBlocks[0]
			read, err := vb.readValue()
			require.NoError(s.T(), err)
			s.Equal(c.expected, read)
		})
	}
}

func (s *variableSuite) TestReadVariableValue_ReadValueFromStdPromoter() {
	cases := []struct {
		desc           string
		config         string
		expectedOutput string
	}{
		{
			desc: "variable with no description",
			config: `variable "string_value" {
}`,
			expectedOutput: `var.string_value
  Enter a value: 
`,
		},
		{
			desc: "variable with description",
			config: `variable "string_value" {
  description = "this is a test"
}`,
			expectedOutput: `var.string_value
  this is a test

  Enter a value: 
`,
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.config,
			})
			mockPromoter := &mockVariableValuePromoter{
				mockInput: "hello",
			}
			stub := gostub.Stub(&valuePromoter, mockPromoter)
			defer stub.Reset()
			config, err := BuildDummyConfig("/", "", nil, nil)
			require.NoError(s.T(), err)
			cfg := config.(*DummyConfig).BaseConfig
			variableBlocks := Blocks[*VariableBlock](cfg)
			vb := variableBlocks[0]
			read := vb.variableValue
			s.NotNil(read)
			s.Equal(cty.StringVal("hello"), *read)
			s.Equal(c.expectedOutput, mockPromoter.sb.String())
		})
	}
}

func (s *variableSuite) TestExecuteBeforePlan_TypeConvert() {
	cases := []struct {
		desc                      string
		variableDef               string
		env                       map[string]string
		expectedVal               cty.Value
		expectedErrorMessageRegex *string
	}{
		{
			desc: "successful conversion",
			variableDef: `variable "test" {
                type = string
            }`,
			env: map[string]string{
				"FT_VAR_test": "true",
			},
			expectedVal: cty.StringVal("true"),
		},
		{
			desc: "unsuccessful conversion",
			variableDef: `variable "test" {
  type = number
}`,
			env: map[string]string{
				"FT_VAR_test": "true",
			},
			expectedErrorMessageRegex: p("incompatible type for var.test, want cty.Number, got cty.Bool"),
		},
	}

	for _, c := range cases {
		s.Run(c.desc, func() {
			for k, v := range c.env {
				s.T().Setenv(k, v)
			}
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.variableDef,
			})
			config, err := BuildDummyConfig("/", "", nil, nil)
			if c.expectedErrorMessageRegex != nil {
				s.Regexp(regexp.MustCompile(*c.expectedErrorMessageRegex), err.Error())
				return
			}
			require.NoError(s.T(), err)
			cfg := config.(*DummyConfig).BaseConfig
			sut := Blocks[*VariableBlock](cfg)[0]
			require.NoError(s.T(), err)
			s.Equal(c.expectedVal, *sut.variableValue)
		})
	}
}

func (s *variableSuite) TestExecuteBeforePlan_Validation() {
	cases := []struct {
		desc                      string
		variableDef               string
		expectedErrorMessageRegex *string
	}{
		{
			desc: "successful single validation",
			variableDef: `variable "test" {
                default = "valid"
				validation {
					condition = var.test == "valid"
					error_message = "var.test must be valid"
				}
            }`,
		},
		{
			desc: "unsuccessful single validation",
			variableDef: `variable "test" {
  				default = "invalid"
				validation {
					condition = var.test == "valid"
					error_message = "var.test must be valid"
				}
			}`,
			expectedErrorMessageRegex: p("invalid value for variable test"),
		},
		{
			desc: "successful multiple validations",
			variableDef: `variable "test" {
                default = "valid"
				validation {
					condition = var.test == "valid"
					error_message = "var.test must be valid"
				}
				validation {
					condition = var.test != ""
					error_message = "var.test must be valid"
				}
            }`,
		},
		{
			desc: "one unsuccessful validation, one successful validation",
			variableDef: `variable "test" {
  				default = "invalid"
				validation {
					condition = var.test == "valid"
					error_message = "var.test must be valid"
				}
				validation {
					condition = var.test == "invalid"
					error_message = "var.test must be invalid"
				}
			}`,
			expectedErrorMessageRegex: p("invalid value for variable test"),
		},
		{
			desc: "function call in condition",
			variableDef: `variable "test" {
                default = "valid"
				validation {
					condition = startswith(var.test, "v")
					error_message = "var.test must starts with v"
				}
            }`,
		},
	}

	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.variableDef,
			})
			_, err := BuildDummyConfig("/", "", nil, nil)
			if c.expectedErrorMessageRegex != nil {
				require.NotNil(s.T(), err)
				s.Regexp(regexp.MustCompile(*c.expectedErrorMessageRegex), err.Error())
				return
			}
			require.NoError(s.T(), err)
		})
	}
}

var _ variableValuePromoter = &mockVariableValuePromoter{}

type mockVariableValuePromoter struct {
	sb        strings.Builder
	mockInput string
}

func (m *mockVariableValuePromoter) printf(format string, a ...any) (n int, err error) {
	return m.sb.WriteString(fmt.Sprintf(format, a...))
}

func (m *mockVariableValuePromoter) scanln(a ...any) (n int, err error) {
	p := a[0].(*string)
	*p = m.mockInput
	return 0, nil
}

func p[T any](input T) *T {
	return &input
}
