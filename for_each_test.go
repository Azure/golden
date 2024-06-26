package golden

import (
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type forEachTestSuite struct {
	suite.Suite
	*testBase
}

func TestForEachTestSuite(t *testing.T) {
	suite.Run(t, new(forEachTestSuite))
}

func (s *forEachTestSuite) SetupTest() {
	s.testBase = newTestBase()
}

func (s *forEachTestSuite) SetupSubTest() {
	s.SetupTest()
}

func (s *forEachTestSuite) TearDownTest() {
	s.teardown()
}

func (s *forEachTestSuite) TearDownSubTest() {
	s.TearDownTest()
}

func (s *forEachTestSuite) TestForEachBlockWithAttributeThatHasDefaultValue() {
	config := `	
	data "dummy" "sample" {
		for_each = toset([1,2,3])
	}
`
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": config,
	})
	c, err := BuildDummyConfig("", "", nil, nil)
	s.NoError(err)
	_, err = RunDummyPlan(c)
	s.NoError(err)
	for _, b := range blocks(c) {
		data := b.(*DummyData)
		s.Equal("default_value", data.AttributeWithDefaultValue)
	}
}

func (s *forEachTestSuite) TestForEachBlockInvolvingVariable() {
	cases := []struct {
		config string
		desc   string
	}{
		{
			// The order of blocks is crucial here. The block with the variable must be defined first
			config: `
data "dummy" "sample" {
	for_each = var.numbers
}

variable "numbers" {
	type = set(number)
}
`,
			desc: "without_validation",
		},
		{
			desc: "with_validation",
			config: `
data "dummy" "sample" {
	for_each = var.numbers
}

variable "numbers" {
	type = set(number)
	validation {
		condition = length(var.numbers) > 0
		error_message = "numbers must not be empty"
	}
}

variable "dummy" {
  type = number
  default = 1
}
`,
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.config,
			})
			c, err := BuildDummyConfig("", "", []CliFlagAssignedVariables{
				NewCliFlagAssignedVariable("numbers", "[1]"),
			}, nil)
			require.NoError(s.T(), err)
			_, err = RunDummyPlan(c)
			s.NoError(err)
		})
	}
}

func (s *forEachTestSuite) TestLocals_locals_as_for_each() {
	code := `
locals {
  numbers = toset([1,2,3])
}

data "dummy" foo {
	for_each = local.numbers
}
`
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": code,
	})
	c, err := BuildDummyConfig("/", "", nil, nil)
	s.NoError(err)
	p, err := RunDummyPlan(c)
	s.NoError(err)
	s.Len(p.Datas, 3)
}

func (s *forEachTestSuite) TestLocals_data_output_as_foreach() {
	code := `
data "dummy" foo {
	data = {
		"1" = "one"
		"2" = "two"
		"3" = "three"
	}
}

resource "dummy" bar {
	for_each = data.dummy.foo.data
}
`
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": code,
	})
	c, err := BuildDummyConfig("/", "", nil, nil)
	s.NoError(err)
	p, err := RunDummyPlan(c)
	s.NoError(err)
	s.Len(p.Resources, 3)
}
