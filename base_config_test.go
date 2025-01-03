package golden

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
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

func (s *baseConfigSuite) TestReadVarsFromDefaultVarFile() {
	cases := []struct {
		desc          string
		filename      string
		content       string
		expected      map[string]VariableValueRead
		expectedError bool
	}{
		{
			desc:     "valid hcl config",
			filename: "terraform.tfvars",
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
			filename: "terraform.tfvars.json",
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
				dslFullName:     "terraform",
				dslAbbreviation: "tf",
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

func (s *baseConfigSuite) TestBaseConfig_ReadVariablesFromDefaultVarFiles() {
	cases := []struct {
		desc         string
		files        map[string]string
		varConfigDir *string
		assert       func(map[string]VariableValueRead)
	}{
		{
			desc: "json vars merge with hcl vars",
			files: map[string]string{
				"/terraform.tfvars": `
string_value = "hello"
bool_value = true
obj_value = {
  name = "John Doe"
  gender = "Male"
}`,
				"/terraform.tfvars.json": `{
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
				"/terraform.tfvars": `
string_value = "hello"
`,
				"/terraform.tfvars.json": `{
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
				"/terraform.tfvars": `
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
				"/terraform.tfvars.json": `{
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
			desc: "default vars in other folder",
			files: map[string]string{
				"/config/terraform.tfvars": `
string_value = "hello"
`,
			},
			varConfigDir: p("/config"),
			assert: func(vars map[string]VariableValueRead) {
				// Assert the variables were read correctly
				s.Len(vars, 1)
				s.Equal("hello", vars["string_value"].Value.AsString())
			},
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			// Create a new BaseConfig
			sut := NewBasicConfig("/", "terraform", "tf", c.varConfigDir, nil, nil)
			vars, err := sut.readVariablesFromDefaultVarFiles()
			// Assert no error occurred
			require.NoError(s.T(), err)
			c.assert(vars)
		})
	}
}

func (s *baseConfigSuite) TestReadVarsFromAutoVarFile() {
	cases := []struct {
		desc         string
		varConfigDir *string
		files        map[string]string
		expected     map[string]VariableValueRead
	}{
		{
			desc: "valid hcl config",
			files: map[string]string{
				"/a.auto.tfvars": `string_value = "hello"`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "valid json config",
			files: map[string]string{
				"/a.auto.tfvars.json": `{
	"string_value": "hello"
}`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "valid hcl config in other folder",
			files: map[string]string{
				"/config/a.auto.tfvars": `string_value = "hello"`,
				"/a.auto.tfvars":        `string_value = "should_not_be_this"`,
			},
			varConfigDir: p("/config"),
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			sut := &BaseConfig{
				basedir:         "/",
				dslAbbreviation: "tf",
				varConfigDir:    c.varConfigDir,
			}
			vars, err := sut.readVariablesFromAutoVarFiles()
			require.NoError(s.T(), err)
			s.Equal(len(c.expected), len(vars))
			for _, varRead := range vars {
				expectedValue, ok := c.expected[varRead.Name]
				require.True(s.T(), ok)
				s.Equal(expectedValue.Name, varRead.Name)
				s.Equal(*expectedValue.Value, *varRead.Value)
			}
		})
	}
}

func (s *baseConfigSuite) TestBaseConfig_ReadVariablesFromAutoVarFiles() {
	cases := []struct {
		desc   string
		files  map[string]string
		assert func(map[string]VariableValueRead)
	}{
		{
			desc: "json vars merge with hcl vars",
			files: map[string]string{
				"/a.auto.tfvars": `
string_value = "hello"
bool_value = true
obj_value = {
  name = "John Doe"
  gender = "Male"
}`,
				"/a.auto.tfvars.json": `{
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
				"/a.auto.tfvars": `
string_value = "hello"
`,
				"/a.auto.tfvars.json": `{
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
			sut := NewBasicConfig("/", "terraform", "tf", nil, nil, nil)
			vars, err := sut.readVariablesFromAutoVarFiles()
			// Assert no error occurred
			require.NoError(s.T(), err)
			c.assert(vars)
		})
	}
}

func (s *baseConfigSuite) TestBaseConfig_ReadAssignedVariables() {
	content := `
	variable "string_value" {
	  type = string
	}
	`

	s.dummyFsWithFiles(map[string]string{
		"test.hcl": content,
	})
	t := s.T()

	config, err := BuildDummyConfig("", "", []CliFlagAssignedVariables{
		CliFlagAssignedVariable{
			varName:  "string_value",
			rawValue: "hello",
		},
	}, nil)
	require.NoError(t, err)
	sut := config.(*DummyConfig).BaseConfig
	variables, err := sut.readCliAssignedVariables()
	require.NoError(s.T(), err)
	s.Len(variables, 1)
	read, ok := variables["string_value"]
	s.True(ok)
	s.Equal(cty.StringVal("hello"), *read.Value)
	s.NoError(read.Error)
}

func (s *baseConfigSuite) TestReadCliAssignedVariables() {
	cases := []struct {
		desc     string
		cliFlags []CliFlagAssignedVariables
		files    map[string]string
		expected map[string]VariableValueRead
	}{
		{
			desc: "CliFlagAssignedVariable",
			cliFlags: []CliFlagAssignedVariables{
				CliFlagAssignedVariable{
					varName:  "string_value",
					rawValue: "hello",
				},
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
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
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "CliFlagAssignedVariableFile-json",
			cliFlags: []CliFlagAssignedVariables{
				NewCliFlagAssignedVariableFile("/test.tfvars.json"),
			},
			files: map[string]string{
				"/test.tfvars.json": `{
	"string_value": "hello"
}`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "CliFlagAssignedVariableFile-json",
			cliFlags: []CliFlagAssignedVariables{
				NewCliFlagAssignedVariableFile("/test.tfvars.json"),
			},
			files: map[string]string{
				"/test.tfvars.json": `{
	"string_value": "hello"
}`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "CliFlagAssignedVariableFile-precedence-0",
			cliFlags: []CliFlagAssignedVariables{
				NewCliFlagAssignedVariable("string_value", "world"),
				NewCliFlagAssignedVariableFile("/test.tfvars.json"),
			},
			files: map[string]string{
				"/test.tfvars.json": `{
	"string_value": "hello"
}`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc: "CliFlagAssignedVariableFile-precedence-1",
			cliFlags: []CliFlagAssignedVariables{
				NewCliFlagAssignedVariableFile("/test.tfvars.json"),
				NewCliFlagAssignedVariableFile("/test.tfvars"),
			},
			files: map[string]string{
				"/test.tfvars.json": `{
	"string_value": "hello"
}`,
				"/test.tfvars": `
string_value = "world"
`,
			},
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("world")), nil),
			},
		},
	}

	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": `variable "string_value" {
}`,
			})
			config, err := BuildDummyConfig("/", "", c.cliFlags, nil)
			require.NoError(s.T(), err)
			sut := config.(*DummyConfig).BaseConfig
			vars, err := sut.readCliAssignedVariables()
			require.NoError(s.T(), err)
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

func TestBaseConfig_OverrideFunctions(t *testing.T) {
	// Define a custom function
	customFunc := function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "input",
				Type: cty.String,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			input := args[0].AsString()
			return cty.StringVal("Hello, " + input), nil
		},
	})

	// Create a new BaseConfig with the custom function in OverrideFunctions
	overrideFunctions := map[string]function.Function{
		"customFunc": customFunc,
	}
	config := NewBasicConfig("/", "dslFullName", "dslAbbreviation", nil, nil, nil)
	config.OverrideFunctions = overrideFunctions

	// Get the evaluation context
	evalContext := config.EmptyEvalContext()

	// Verify that the custom function is included in the evaluation context
	assert.Contains(t, evalContext.Functions, "customFunc")

	// Test the custom function
	result, err := evalContext.Functions["customFunc"].Call([]cty.Value{cty.StringVal("World")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("Hello, World"), result)
}
