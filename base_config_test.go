package golden

import (
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
	"testing"
)

type baseConfigSuite struct {
	suite.Suite
	*testBase
}

func TestBaseConfigSuiteSuite(t *testing.T) {
	suite.Run(t, new(baseConfigSuite))
}

func (s *baseConfigSuite) SetupTest() {
	s.testBase = newTestBase()
}

func (s *baseConfigSuite) TearDownTest() {
	s.teardown()
}

func (s *baseConfigSuite) TestReadVarsFromVarFile() {
	cases := []struct {
		desc          string
		filename      string
		content       string
		expected      map[string]VariableValueRead
		expectedError bool
	}{
		{
			desc:     "valid hcl config",
			filename: "test.ftvars",
			content: `string_value = "hello"
bool_value = true
obj_value = {
  name = "John Doe"
  gender = "Male"
}`,
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
				"bool_value":   NewVariableValueRead("bool_value", p(cty.True), nil),
				"obj_value": NewVariableValueRead("obj_value", p(cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("John Doe"),
					"gender": cty.StringVal("Male"),
				})), nil),
			},
		},
		{
			desc:     "valid json config",
			filename: "test.ftvars.json",
			content: `{
	"string_value": "hello",
    "bool_value": true,
	"obj_value": {
		"name": "John Doe",
		"gender": "Male"
    }
}`,
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
				"bool_value":   NewVariableValueRead("bool_value", p(cty.True), nil),
				"obj_value": NewVariableValueRead("obj_value", p(cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("John Doe"),
					"gender": cty.StringVal("Male"),
				})), nil),
			},
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			sut := &BaseConfig{
				dslAbbreviation: "ft",
			}
			vars, err := sut.ReadVariablesFromSingleVarFile([]byte(c.content), c.filename)
			if c.expectedError {
				s.NotNil(err)
				return
			}
			s.Equal(len(c.expected), len(vars))
			for _, varRead := range vars {
				expectedValue, ok := c.expected[varRead.Name]
				require.True(s.T(), ok)
				s.Equal(expectedValue.Name, varRead.Name)
				s.Equal(*expectedValue.Value, *varRead.Value)
				s.Equal(expectedValue.Error, varRead.Error)
			}
		})
	}
}
