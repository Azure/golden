package golden

import (
	"math/big"
	"testing"

	"github.com/ahmetb/go-linq/v3"
	"github.com/hashicorp/hcl/v2"
	"github.com/prashantv/gostub"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/zclconf/go-cty/cty"
)

type TestData interface {
	PlanBlock
	Data()
}

type BaseData struct{}

func (db *BaseData) BlockType() string {
	return "data"
}

func (db *BaseData) Data() {}

func (db *BaseData) AddressLength() int { return 3 }

func (db *BaseData) CanExecutePrePlan() bool {
	return false
}

type TestResource interface {
	PlanBlock
	ApplyBlock
	Resource()
}

type BaseResource struct{}

func (rb *BaseResource) BlockType() string {
	return "resource"
}

func (rb *BaseResource) Resource() {}

func (rb *BaseResource) AddressLength() int {
	return 3
}

func (rb *BaseResource) CanExecutePrePlan() bool {
	return false
}

var _ TestData = &DummyData{}

type TopNestedBlock struct {
	Name               string              `hcl:"name"`
	SecondNestedBlocks []SecondNestedBlock `hcl:"second_nested_block,block"`
}

type SecondNestedBlock struct {
	Id   int    `hcl:"id"`
	Name string `hcl:"name"`
}

type DummyData struct {
	*BaseData
	*BaseBlock
	Tags                      map[string]string `json:"data" hcl:"data,optional"`
	AttributeWithDefaultValue string            `json:"attribute" hcl:"attribute,optional" default:"default_value"`
	TopNestedBlocks           []TopNestedBlock  `json:"top_nested_block" hcl:"top_nested_block,block"`
}

func (d *DummyData) Type() string {
	return "dummy"
}

func (d *DummyData) ExecuteDuringPlan() error {
	return nil
}

var _ TestResource = &DummyResource{}

type DummyResource struct {
	*BaseBlock
	*BaseResource
	Tags map[string]string `json:"tags" hcl:"tags,optional"`
}

func (d *DummyResource) Type() string {
	return "dummy"
}

func (d *DummyResource) ExecuteDuringPlan() error {
	return nil
}

func (d *DummyResource) Apply() error {
	return nil
}

type blockTestSuite struct {
	suite.Suite
	*testBase
}

func TestBlockSuite(t *testing.T) {
	suite.Run(t, new(blockTestSuite))
}

func (s *blockTestSuite) SetupTest() {
	s.testBase = newTestBase()
}

func (s *blockTestSuite) TearDownTest() {
	s.teardown()
}

func (s *blockTestSuite) Test_DependsOn() {
	code := `data "dummy" this {
}

  resource "dummy" this {
  depends_on = [data.dummy.this]
}`
	s.dummyFsWithFiles(map[string]string{
		"test.hcl": code,
	})
	t := s.T()
	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	_, err = RunDummyPlan(config)
	require.NoError(t, err)
	children, err := config.GetChildren("data.dummy.this")
	require.NoError(t, err)
	_, ok := children["resource.dummy.this"]
	s.True(ok)
}

func (s *blockTestSuite) Test_DependsOnMustBeListOfBlockAddress() {
	cases := []struct {
		desc string
		code string
	}{
		{
			desc: "not a list",
			code: `data "dummy" this {
}

  resource "dummy" this {
  depends_on = data.dummy.this
}`,
		},
		{
			desc: "list element is not block address",
			code: `data "dummy" this {
}

  resource "dummy" this {
  depends_on = ["hello"]
}`,
		},
	}

	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.code,
			})

			config, err := BuildDummyConfig("", "", nil, nil)
			require.NoError(s.T(), err)
			_, err = RunDummyPlan(config)
			s.NotNil(err)
		})
	}
}

func Test_NestedBlock(t *testing.T) {
	code := `data "dummy" this {
	top_nested_block {
	  name = "name"
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 1)
	assert.Equal(t, "name", dataBlock.TopNestedBlocks[0].Name)
}

func Test_NestedBlock_Dynamic(t *testing.T) {
	code := `data "dummy" this {
	dynamic "top_nested_block" {
      for_each = toset([1])
	  content {
        name = "name"
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 1)
	assert.Equal(t, "name", dataBlock.TopNestedBlocks[0].Name)
}

func Test_NestedBlock_DynamicUseForEach(t *testing.T) {
	code := `data "dummy" this {
	dynamic "top_nested_block" {
      for_each = toset(["hello"])
	  content {
        name = top_nested_block.value
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 1)
	assert.Equal(t, "hello", dataBlock.TopNestedBlocks[0].Name)
}

func Test_NestedBlock_DynamicMultipleDecode(t *testing.T) {
	code := `data "dummy" this {
	dynamic "top_nested_block" {
      for_each = toset(["hello"])
	  content {
        name = top_nested_block.value
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 1)
	assert.Equal(t, "hello", dataBlock.TopNestedBlocks[0].Name)
}

func Test_NestedBlock_DynamicAndNonDynamic(t *testing.T) {
	code := `data "dummy" this {
	top_nested_block {
	  name = "hello"
    }
	dynamic "top_nested_block" {
      for_each = toset(["world"])
	  content {
        name = top_nested_block.value
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 2)
	assert.Equal(t, "hello", dataBlock.TopNestedBlocks[0].Name)
	assert.Equal(t, "world", dataBlock.TopNestedBlocks[1].Name)
}

func Test_NestedBlock_DynamicWithMultipleElementes(t *testing.T) {
	code := `data "dummy" this {
	dynamic "top_nested_block" {
      for_each = toset(["hello", "world"])
	  content {
        name = top_nested_block.value
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 2)
	assert.True(t, linq.From(dataBlock.TopNestedBlocks).AnyWith(func(i interface{}) bool {
		return i.(TopNestedBlock).Name == "hello"
	}))
	assert.True(t, linq.From(dataBlock.TopNestedBlocks).AnyWith(func(i interface{}) bool {
		return i.(TopNestedBlock).Name == "world"
	}))
}

func Test_NestedBlock_DynamicInsideDynamic(t *testing.T) {
	code := `data "dummy" this {
	dynamic "top_nested_block" {
      for_each = toset(["hello"])
	  content {
        name = top_nested_block.value
		dynamic "second_nested_block" {
		  for_each = toset([1])
		  content {
			id = second_nested_block.value
			name = top_nested_block.value
		  }
		}
	  }
	}
}
`
	mockFs := afero.NewMemMapFs()
	stub := gostub.Stub(&testFsFactory, func() afero.Fs {
		return mockFs
	})
	defer stub.Reset()
	_ = afero.WriteFile(mockFs, "test.hcl", []byte(code), 0644)

	config, err := BuildDummyConfig("", "", nil, nil)
	require.NoError(t, err)
	block := config.GetVertices()["data.dummy.this"].(Block)
	err = Decode(block)
	require.NoError(t, err)
	dataBlock := block.(*DummyData)
	assert.Len(t, dataBlock.TopNestedBlocks, 1)
	assert.Len(t, dataBlock.TopNestedBlocks[0].SecondNestedBlocks, 1)
	secondNesteBlock := dataBlock.TopNestedBlocks[0].SecondNestedBlocks[0]
	assert.Equal(t, "hello", secondNesteBlock.Name)
	assert.Equal(t, 1, secondNesteBlock.Id)
}

func Test_LocalBlocksValueShouldBeAFlattenObject(t *testing.T) {
	numberVal := cty.NumberVal(big.NewFloat(1))
	stringVal := cty.StringVal("hello world")
	locals := []Block{
		&LocalBlock{
			BaseBlock: &BaseBlock{
				name: "number_value",
			},
			LocalValue: numberVal,
		},
		&LocalBlock{
			BaseBlock: &BaseBlock{
				name: "string_value",
			},
			LocalValue: stringVal,
		},
	}

	values := SingleValues(castBlock[SingleValueBlock](locals))
	assert.True(t, AreCtyValuesEqual(numberVal, values.GetAttr("number_value")))
	assert.True(t, AreCtyValuesEqual(stringVal, values.GetAttr("string_value")))
}

func TestBlockToString(t *testing.T) {

	cases := []struct {
		desc     string
		b        Block
		expected string
	}{
		{
			desc:     "normal_block_should_return_json_marshal",
			b:        fakeBlock{Input: "input"},
			expected: `{"input":"input"}`,
		},
		{
			desc:     "block_implement_stringer_interface_should_return_customized_string",
			b:        customizedStringBlock{fakeBlock{Input: "input"}},
			expected: `input`,
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			actual := BlockToString(c.b)
			assert.Equal(t, c.expected, actual)
		})
	}
}

type fakeBlock struct {
	Input string `json:"input"`
}

type customizedStringBlock struct {
	fakeBlock
}

func (c customizedStringBlock) String() string {
	return c.Input
}

func (c fakeBlock) Config() Config { panic("implement me") }

func (c fakeBlock) Id() string {
	panic("implement me")
}

func (c fakeBlock) Name() string {
	panic("implement me")
}

func (c fakeBlock) Type() string {
	panic("implement me")
}

func (c fakeBlock) BlockType() string {
	panic("implement me")
}

func (c fakeBlock) Address() string {
	panic("implement me")
}

func (c fakeBlock) HclBlock() *HclBlock {
	panic("implement me")
}

func (c fakeBlock) EvalContext() *hcl.EvalContext {
	panic("implement me")
}

func (c fakeBlock) BaseValues() map[string]cty.Value {
	panic("implement me")
}

func (c fakeBlock) PreConditionCheck(context *hcl.EvalContext) ([]PreCondition, error) {
	panic("implement me")
}

func (c fakeBlock) AddressLength() int {
	panic("implement me")
}

func (c fakeBlock) CanExecutePrePlan() bool {
	panic("implement me")
}

func (c fakeBlock) getDownstreams() []Block {
	panic("implement me")
}

func (c fakeBlock) getForEach() *ForEach {
	panic("implement me")
}

func (c fakeBlock) markExpanded() {
	panic("implement me")
}

func (c fakeBlock) isReadyForRead() bool {
	panic("implement me")
}

func (c fakeBlock) markReady() {
	panic("implement me")
}

func (c fakeBlock) expandable() bool {
	panic("implement me")
}

var _ Block = fakeBlock{}
