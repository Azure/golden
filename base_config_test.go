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

func (s *baseConfigSuite) SetupSubTest() {
	s.SetupTest()
}

func (s *baseConfigSuite) TearDownTest() {
	s.teardown()
}

func (s *baseConfigSuite) TearDownSubTest() {
	s.TearDownTest()
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

func (s *baseConfigSuite) TestBaseConfig_ReadVariablesFromVarFiles() {
	cases := []struct {
		desc   string
		files  map[string]string
		assert func(map[string]VariableValueRead)
	}{
		{
			desc: "json vars merge with hcl vars",
			files: map[string]string{
				"/test.ftvars": `
string_value = "hello"
bool_value = true
obj_value = {
  name = "John Doe"
  gender = "Male"
}`,
				"/test.ftvars.json": `{
    "bool_value": true,
	"obj_value": {
		"name": "John Doe",
		"gender": "Male"
    }
}`,
			},
			assert: func(vars map[string]VariableValueRead) {
				// Assert the variables were read correctly
				s.Len(vars, 3)
				s.Equal("hello", vars["string_value"].Value.AsString())
				s.True(vars["bool_value"].Value.True())
				s.Equal("John Doe", vars["obj_value"].Value.GetAttr("name").AsString())
				s.Equal("Male", vars["obj_value"].Value.GetAttr("gender").AsString())
			},
		},
		{
			desc: "json vars should take precedence ",
			files: map[string]string{
				"/test.ftvars": `
string_value = "hello"
`,
				"/test.ftvars.json": `{
    "string_value": "world"
}`,
			},
			assert: func(vars map[string]VariableValueRead) {
				// Assert the variables were read correctly
				s.Len(vars, 1)
				s.Equal("world", vars["string_value"].Value.AsString())
			},
		},
		{
			desc: "hcl config only",
			files: map[string]string{
				"/test.ftvars": `
string_value = "hello"
`,
			},
			assert: func(vars map[string]VariableValueRead) {
				// Assert the variables were read correctly
				s.Len(vars, 1)
				s.Equal("hello", vars["string_value"].Value.AsString())
			},
		},
		{
			desc: "json config only",
			files: map[string]string{
				"/test.ftvars.json": `{
    "string_value": "world"
}`,
			},
			assert: func(vars map[string]VariableValueRead) {
				// Assert the variables were read correctly
				s.Len(vars, 1)
				s.Equal("world", vars["string_value"].Value.AsString())
			},
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			// Create a new BaseConfig
			sut := NewBasicConfig("/", "test", "ft", nil)
			vars, err := sut.ReadVariablesFromVarFiles()
			// Assert no error occurred
			require.NoError(s.T(), err)
			c.assert(vars)
		})
	}
}
