package golden

import (
	"fmt"
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
	Id                int                `hcl:"id"`
	Name              string             `hcl:"name"`
	ThirdNestedBlocks []ThirdNestedBlock `hcl:"third_nested_block,block"`
}

type ThirdNestedBlock struct {
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
	Tags         map[string]string   `json:"tags" hcl:"tags,optional"`
	NestedBlocks []SecondNestedBlock `hcl:"nested_block,block"`
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

var _ ApplyBlock = &PureApplyBlock{}
var _ CustomDecode = &PureApplyBlock{}

type PureApplyBlock struct {
	*BaseBlock
}

func (p *PureApplyBlock) Decode(block *HclBlock, context *hcl.EvalContext) error {
	return nil
}

func (p *PureApplyBlock) Type() string {
	return "one"
}

func (p *PureApplyBlock) BlockType() string {
	return "pure_apply"
}

func (p *PureApplyBlock) AddressLength() int {
	return 3
}

func (p *PureApplyBlock) CanExecutePrePlan() bool {
	return false
}

func (p *PureApplyBlock) Apply() error {
	return nil
}

var _ ApplyBlock = &PureApplyBlock2{}
var _ CustomDecode = &PureApplyBlock2{}

type PureApplyBlock2 struct {
	*BaseBlock
	decoded bool
}

func (p *PureApplyBlock2) Decode(block *HclBlock, context *hcl.EvalContext) error {
	p.decoded = true
	return nil
}

func (p *PureApplyBlock2) Type() string {
	return "two"
}

func (p *PureApplyBlock2) BlockType() string {
	return "pure_apply"
}

func (p *PureApplyBlock2) AddressLength() int {
	return 3
}

func (p *PureApplyBlock2) CanExecutePrePlan() bool {
	return false
}

func (p *PureApplyBlock2) Apply() error {
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
	cases := []struct {
		desc     string
		tfConfig string
	}{
		{
			desc: "simple case",
			tfConfig: `data "dummy" this {
}

  resource "dummy" this {
  depends_on = [data.dummy.this]
}`,
		},
		{
			desc: "with white space",
			tfConfig: `data "dummy" this {
}

  resource "dummy" this {
  depends_on = [ data.dummy.this ]
}`,
		},
		{
			desc: "with tab",
			tfConfig: fmt.Sprintf(`data "dummy" this {
}

  resource "dummy" this {
  depends_on = [%sdata.dummy.this%s]
}`, "\t", "\t"),
		},
		{
			desc: "with new line",
			tfConfig: fmt.Sprintf(`data "dummy" this {
}

  resource "dummy" this {
  depends_on = [%sdata.dummy.this,%s]
}`, "\n", "\r\n"),
		},
		{
			desc: "multiple items mixed with tab, white space, new line",
			tfConfig: fmt.Sprintf(`data "dummy" this {
}

  resource "dummy" this {
  depends_on = [%sdata.dummy.this,%s]
}`, "\n\t ", " \t\r\n"),
		},
		{
			desc: "multiple items with new line",
			tfConfig: `data "dummy" this {
}

  resource "dummy" this {
  depends_on = [
    data.dummy.this,
    data.dummy.this,
  ]
}`,
		},
	}
	for _, c := range cases {
		s.Run(c.desc, func() {
			s.dummyFsWithFiles(map[string]string{
				"test.hcl": c.tfConfig,
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

		})
	}
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

func Test_NestedBlock_DynamicWithMultipleElements(t *testing.T) {
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

func Test_NestedBlock_DynamicInsideStatic(t *testing.T) {
	code := `data "dummy" this {
	top_nested_block {
      name = "hello"
	  dynamic "second_nested_block" {
		for_each = toset([1])
		content {
		  id = second_nested_block.value
		  name = "hello"
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

func Test_NestedBlock_DeepMixedStaticAndDynamic(t *testing.T) {
	code := `data "dummy" this {
	top_nested_block {
      name = "hello"
	  dynamic "second_nested_block" {
		for_each = toset([1])
		content {
		  id = second_nested_block.value
		  name = "second_nested_block0"
          third_nested_block {
            name = second_nested_block.value
          }
        }
      }
      second_nested_block {
        id = 2
        name = "world"
        dynamic "third_nested_block" {
          for_each = [1,2]
          content {
			name = "world${third_nested_block.value}"
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
	topNestedBlock := dataBlock.TopNestedBlocks[0]
	assert.Equal(t, "hello", topNestedBlock.Name)

	assert.Len(t, topNestedBlock.SecondNestedBlocks, 2)
	secondNestedBlock0 := topNestedBlock.SecondNestedBlocks[0]
	assert.Equal(t, 1, secondNestedBlock0.Id)
	assert.Equal(t, "second_nested_block0", secondNestedBlock0.Name)
	assert.Len(t, secondNestedBlock0.ThirdNestedBlocks, 1)
	assert.Equal(t, "1", secondNestedBlock0.ThirdNestedBlocks[0].Name)

	secondNestedBlock1 := topNestedBlock.SecondNestedBlocks[1]
	assert.Equal(t, 2, secondNestedBlock1.Id)
	assert.Equal(t, "world", secondNestedBlock1.Name)
	assert.Len(t, secondNestedBlock1.ThirdNestedBlocks, 2)
	assert.Equal(t, "world1", secondNestedBlock1.ThirdNestedBlocks[0].Name)
	assert.Equal(t, "world2", secondNestedBlock1.ThirdNestedBlocks[1].Name)
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

func TestMultipleInstanceDataBlockWithNestedBlock(t *testing.T) {
	code := `data "dummy" this {
    for_each = toset([1,2])
	dynamic "top_nested_block" {
		for_each = range(each.value)
		content {
        	name = "name${top_nested_block.value}"
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
	_, err = RunDummyPlan(config)
	require.NoError(t, err)
}

func TestPureApplyBlockDependOnPureApplyBlock(t *testing.T) {
	code := `
    pure_apply "one" this {
	}

	pure_apply "two" this {
	  depends_on = [pure_apply.one.this]
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
	_, err = RunDummyPlan(config)
	require.NoError(t, err)
	for _, b := range config.GetVertices() {
		if pb, ok := b.(*PureApplyBlock2); ok {
			assert.True(t, pb.decoded)
			return
		}
	}
	t.Fatal("should got PureApplyBlock2")
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
