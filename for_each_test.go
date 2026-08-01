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

func (s *forEachTestSuite) TestStaticNestedBlocksUseInstanceContext() {
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": `
resource "dummy" "expanded" {
  for_each = {
    one   = "first"
    two   = "second"
    three = "third"
  }

  nested_block {
    id   = 1
    name = each.value

    third_nested_block {
      name = each.value
    }
  }
}
`,
	})

	hclBlocks, err := loadHclBlocks(false, "")
	require.NoError(s.T(), err)
	require.Len(s.T(), hclBlocks, 1)
	sourceNestedAttribute := hclBlocks[0].NestedBlocks()[0].Attributes()["name"].Attribute

	config, err := NewDummyConfig("", nil, hclBlocks, nil)
	require.NoError(s.T(), err)
	resources := Blocks[TestResource](config)
	require.Len(s.T(), resources, 3)
	for i := 0; i < len(resources); i++ {
		for j := i + 1; j < len(resources); j++ {
			left := resources[i].HclBlock()
			right := resources[j].HclBlock()
			s.NotSame(left.Block, right.Block)
			s.NotSame(left.Body, right.Body)
			s.NotSame(left.Body.Blocks[0], right.Body.Blocks[0])
			s.NotSame(left.Body.Blocks[0].Body.Blocks[0], right.Body.Blocks[0].Body.Blocks[0])
		}
	}

	plan, err := RunDummyPlan(config)
	require.NoError(s.T(), err)
	require.Len(s.T(), plan.Resources, 3)

	got := make(map[string][2]string, 3)
	for _, resource := range plan.Resources {
		block := resource.(*DummyResource)
		require.Len(s.T(), block.NestedBlocks, 1)
		require.Len(s.T(), block.NestedBlocks[0].ThirdNestedBlocks, 1)
		got[block.Address()] = [2]string{
			block.NestedBlocks[0].Name,
			block.NestedBlocks[0].ThirdNestedBlocks[0].Name,
		}
	}

	s.Equal(map[string][2]string{
		"resource.dummy.expanded[one]":   {"first", "first"},
		"resource.dummy.expanded[two]":   {"second", "second"},
		"resource.dummy.expanded[three]": {"third", "third"},
	}, got)
	s.Same(sourceNestedAttribute, hclBlocks[0].Body.Blocks[0].Body.Attributes["name"])
}

func (s *forEachTestSuite) TestDynamicNestedBlocksUseInstanceContext() {
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": `
resource "dummy" "expanded" {
  for_each = {
    one   = "first"
    two   = "second"
    three = "third"
  }

  dynamic "nested_block" {
    for_each = [each.value]
    content {
      id   = 1
      name = nested_block.value

      third_nested_block {
        name = each.value
      }
    }
  }
}
`,
	})

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(s.T(), err)
	plan, err := RunDummyPlan(config)
	require.NoError(s.T(), err)
	require.Len(s.T(), plan.Resources, 3)

	got := make(map[string][2]string, 3)
	for _, resource := range plan.Resources {
		block := resource.(*DummyResource)
		require.Len(s.T(), block.NestedBlocks, 1)
		require.Len(s.T(), block.NestedBlocks[0].ThirdNestedBlocks, 1)
		got[block.Address()] = [2]string{
			block.NestedBlocks[0].Name,
			block.NestedBlocks[0].ThirdNestedBlocks[0].Name,
		}
	}

	s.Equal(map[string][2]string{
		"resource.dummy.expanded[one]":   {"first", "first"},
		"resource.dummy.expanded[two]":   {"second", "second"},
		"resource.dummy.expanded[three]": {"third", "third"},
	}, got)
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
