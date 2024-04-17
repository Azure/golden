package golden

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
	"math/big"
	"testing"
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

type DummyData struct {
	*BaseData
	*BaseBlock
	Tags                      map[string]string `json:"data" hcl:"data,optional"`
	AttributeWithDefaultValue string            `json:"attribute" hcl:"attribute,optional" default:"default_value"`
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
