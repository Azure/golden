package golden

import (
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
	"path/filepath"
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
		desc   string
		files  map[string]string
		assert func(map[string]VariableValueRead)
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
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(c.files)
			// Create a new BaseConfig
			sut := NewBasicConfig("/", "terraform", "tf", nil, nil)
			vars, err := sut.readVariablesFromDefaultVarFiles()
			// Assert no error occurred
			require.NoError(s.T(), err)
			c.assert(vars)
		})
	}
}

func (s *baseConfigSuite) TestReadVarsFromAutoVarFile() {
	cases := []struct {
		desc     string
		filename string
		content  string
		expected map[string]VariableValueRead
	}{
		{
			desc:     "valid hcl config",
			filename: "a.auto.tfvars",
			content:  `string_value = "hello"`,
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
		{
			desc:     "valid json config",
			filename: "a.auto.tfvars.json",
			content: `{
	"string_value": "hello"
}`,
			expected: map[string]VariableValueRead{
				"string_value": NewVariableValueRead("string_value", p(cty.StringVal("hello")), nil),
			},
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				filepath.Join("/", c.filename): c.content,
			})
			sut := &BaseConfig{
				basedir:         "/",
				dslAbbreviation: "tf",
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
			sut := NewBasicConfig("/", "terraform", "tf", nil, nil)
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
				CliFlagAssignedVariableFile{
					varFileName: "/test.tfvars.json",
				},
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
				CliFlagAssignedVariableFile{
					varFileName: "/test.tfvars.json",
				},
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
				CliFlagAssignedVariable{
					varName:  "string_value",
					rawValue: "world",
				},
				CliFlagAssignedVariableFile{
					varFileName: "/test.tfvars.json",
				},
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
				CliFlagAssignedVariableFile{
					varFileName: "/test.tfvars.json",
				},
				CliFlagAssignedVariableFile{
					varFileName: "/test.tfvars",
				},
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
