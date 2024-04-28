package golden

import (
	"encoding/json"
	"fmt"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/lonegunmanb/go-defaults"
	"github.com/zclconf/go-cty/cty"
	"reflect"
	"strings"
)

type BlockType interface {
	BlockType() string
}

type Block interface {
	Id() string
	Name() string
	Type() string
	BlockType() string
	Address() string
	HclBlock() *HclBlock
	EvalContext() *hcl.EvalContext
	BaseValues() map[string]cty.Value
	PreConditionCheck(*hcl.EvalContext) ([]PreCondition, error)
	AddressLength() int
	CanExecutePrePlan() bool
	Config() Config
	getDownstreams() []Block
	getForEach() *ForEach
	markExpanded()
	isReadyForRead() bool
	markReady()
	expandable() bool
}

func BlockToString(f Block) string {
	if s, ok := f.(fmt.Stringer); ok {
		return s.String()
	}
	marshal, _ := json.Marshal(f)
	return string(marshal)
}

var metaAttributeNames = hashset.New("for_each", "rule_ids")
var metaNestedBlockNames = hashset.New("precondition", "dynamic")

func Decode(b Block) error {
	hb := b.HclBlock()
	evalContext := b.EvalContext()
	if customDecode, ok := b.(CustomDecode); ok {
		return customDecode.Decode(hb, evalContext)
	}
	if baseDecode, ok := b.(BaseDecode); ok {
		err := baseDecode.BaseDecode(hb, evalContext)
		if err != nil {
			return err
		}
	}
	diag := gohcl.DecodeBody(cleanBodyForDecode(hb.Body), evalContext, b)
	if diag.HasErrors() {
		return diag
	}
	err := decodeDynamicNestedBlock(b, b.HclBlock().Block, evalContext)
	if err != nil {
		return err
	}
	// we need set defaults again, since gohcl.DecodeBody might erase default value set on those attribute has null values.
	defaults.SetDefaults(b)
	return nil
}

func decodeDynamicNestedBlock(b any, block *hclsyntax.Block, evalContext *hcl.EvalContext) error {
	// Iterate over the blocks in the HCL block
	for _, block := range block.Body.Blocks {
		if block.Type != "dynamic" {
			continue
		}
		forEachAttr, ok := block.Body.Attributes["for_each"]
		if !ok {
			return fmt.Errorf("`dynamic` block must has `for_each`")
		}
		forEachValue, diag := forEachAttr.Expr.Value(evalContext)
		if diag.HasErrors() {
			return diag
		}
		if !forEachValue.CanIterateElements() {
			return fmt.Errorf("incorrect type for `for_each`, must be collection")
		}

		// Get the field in the parent block to append to
		blockType := block.Labels[0]
		parentField, ok := getFieldByTag(reflect.ValueOf(b).Elem(), blockType)
		if !ok {
			return fmt.Errorf("no such field: %s in block", blockType)
		}

		// Ensure we're working with a slice
		if parentField.Kind() != reflect.Slice {
			return fmt.Errorf("field: %s is not a slice", blockType)
		}

		// Append the parsed structs to the corresponding field in the parent block
		for _, innerBlock := range block.Body.Blocks {
			if innerBlock.Type != "content" {
				return fmt.Errorf("dynamic should contains `content` block only: %s", innerBlock.DefRange().String())
			}
			iterator := forEachValue.ElementIterator()
			iteratorName := blockType
			for iterator.Next() {
				key, value := iterator.Element()
				newContext := evalContext.NewChild()
				newContext.Variables = map[string]cty.Value{
					iteratorName: cty.ObjectVal(map[string]cty.Value{
						"key":   key,
						"value": value,
					}),
				}
				err := decodeContentBlock(parentField, innerBlock, newContext)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// getFieldByTag returns the struct field with the given tag
func getFieldByTag(v reflect.Value, tag string) (reflect.Value, bool) {
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		hclTag := field.Tag.Get("hcl")
		hclTagContents := strings.Split(hclTag, ",")
		if hclTagContents[0] == tag {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func decodeContentBlock(parentField reflect.Value, innerBlock *hclsyntax.Block, evalContext *hcl.EvalContext) error {
	// Decode the inner block into a new struct
	newStructPtr := reflect.New(parentField.Type().Elem())
	diags := gohcl.DecodeBody(bodyWithDynamicNestedBlocks(innerBlock.Body), evalContext, newStructPtr.Interface())
	if diags.HasErrors() {
		return diags
	}
	if err := decodeDynamicNestedBlock(newStructPtr.Interface(), innerBlock, evalContext); err != nil {
		return err
	}

	// Append the new struct to the parent field
	parentField.Set(reflect.Append(parentField, newStructPtr.Elem()))
	return nil
}

func cleanBodyForDecode(hb *hclsyntax.Body) *hclsyntax.Body {
	// Create a new hclsyntax.Body
	newBody := &hclsyntax.Body{
		Attributes: make(hclsyntax.Attributes),
	}

	// Iterate over the attributes of the original body
	for attrName, attr := range hb.Attributes {
		if metaAttributeNames.Contains(attrName) {
			continue
		}
		newBody.Attributes[attrName] = attr
	}

	for _, nb := range hb.Blocks {
		if metaNestedBlockNames.Contains(nb.Type) {
			continue
		}
		newBody.Blocks = append(newBody.Blocks, nb)
	}

	return newBody
}

func bodyWithDynamicNestedBlocks(hb *hclsyntax.Body) *hclsyntax.Body {
	newBody := &hclsyntax.Body{
		Attributes: make(hclsyntax.Attributes),
	}

	// Iterate over the attributes of the original body
	for attrName, attr := range hb.Attributes {
		newBody.Attributes[attrName] = attr
	}

	for _, nb := range hb.Blocks {
		if nb.Type != "dynamic" {
			newBody.Blocks = append(newBody.Blocks, nb)
		}
	}

	return newBody
}

func SingleValues(blocks []SingleValueBlock) cty.Value {
	if len(blocks) == 0 {
		return cty.EmptyObjectVal
	}
	res := map[string]cty.Value{}
	for _, b := range blocks {
		res[b.Name()] = b.Value()
	}
	return cty.ObjectVal(res)
}

func Values[T Block](blocks []T) cty.Value {
	if len(blocks) == 0 {
		return cty.EmptyObjectVal
	}
	res := map[string]cty.Value{}
	valuesMap := map[string]map[string]cty.Value{}

	for _, b := range blocks {
		values, exists := valuesMap[b.Type()]
		if !exists {
			values = map[string]cty.Value{}
			valuesMap[b.Type()] = values
		}
		blockVal := blockToCtyValue(b)
		forEach := b.getForEach()
		if forEach == nil {
			values[b.Name()] = blockVal
		} else {
			m, ok := values[b.Name()]
			if !ok {
				m = cty.MapValEmpty(cty.EmptyObject)
			}
			nm := m.AsValueMap()
			if nm == nil {
				nm = make(map[string]cty.Value)
			}
			nm[CtyValueToString(forEach.key)] = blockVal
			values[b.Name()] = cty.MapVal(nm)
		}
		valuesMap[b.Type()] = values
	}
	for t, m := range valuesMap {
		res[t] = cty.ObjectVal(m)
	}
	return cty.ObjectVal(res)
}

func blockToCtyValue(b Block) cty.Value {
	blockValues := map[string]cty.Value{}
	baseCtyValues := b.BaseValues()
	ctyValues := Value(b) //.Values()
	for k, v := range ctyValues {
		blockValues[k] = v
	}
	for k, v := range baseCtyValues {
		blockValues[k] = v
	}
	blockVal := cty.ObjectVal(blockValues)
	return blockVal
}

func concatLabels(labels []string) string {
	sb := strings.Builder{}
	for i, l := range labels {
		if l == "" {
			continue
		}
		sb.WriteString(l)
		if i != len(labels)-1 {
			sb.WriteString(".")
		}
	}
	return sb.String()
}

func blockAddress(b *HclBlock) string {
	if b == nil {
		return ""
	}
	sb := strings.Builder{}
	sb.WriteString(b.Block.Type)
	sb.WriteString(".")
	sb.WriteString(concatLabels(b.Block.Labels))
	if b.ForEach != nil {
		sb.WriteString(fmt.Sprintf("[%s]", CtyValueToString(b.ForEach.key)))
	}
	return sb.String()
}

// Not all `local` expression could be evaluated before for_each expansion, so we need to try to evaluate them.
func prePlan(b Block) error {
	l, ok := b.(PrePlanBlock)
	if !ok {
		return nil
	}
	err := l.ExecuteBeforePlan()
	if err != nil {
		return err
	}
	b.markReady()
	return nil
}
