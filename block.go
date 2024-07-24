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

type BlockCustomizedRefType interface {
	CustomizedRefType() string
}

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

var MetaAttributeNames = hashset.New("for_each", "depends_on")
var MetaNestedBlockNames = hashset.New("precondition", "dynamic")

func Decode(b Block) error {
	hb := b.HclBlock()
	if err := verifyDependsOn(b); err != nil {
		return err
	}
	zeroBlock(b)
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

	expandedHb, err := hb.ExpandDynamicBlocks(evalContext)
	if err != nil {
		return err
	}

	diag := gohcl.DecodeBody(cleanBodyForDecode(expandedHb.Body), evalContext, b)
	if diag.HasErrors() {
		return diag
	}
	// we need set defaults again, since gohcl.DecodeBody might erase default value set on those attribute has null values.
	defaults.SetDefaults(b)
	return nil
}

func zeroBlock(b Block) {
	nb, _ := wrapBlock(b.Config(), b.HclBlock())
	v := reflect.ValueOf(b).Elem()
	vnb := reflect.ValueOf(nb).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldNb := vnb.Field(i)
		if !field.CanSet() {
			continue
		}

		tag := v.Type().Field(i).Tag
		if _, ok := tag.Lookup("hcl"); ok || strings.Contains(string(tag), "attribute") {
			field.Set(fieldNb)
		}
	}
}

func verifyDependsOn(b Block) error {
	dependsOn, ok := b.HclBlock().Attributes()["depends_on"]
	if !ok {
		return nil
	}
	exprString := strings.TrimSpace(dependsOn.ExprString())
	if !strings.HasPrefix(exprString, "[") && !strings.HasSuffix(exprString, "]") {
		return fmt.Errorf("`depends_on` must be a list of block address")
	}
	elements := strings.Split(strings.TrimSuffix(strings.TrimPrefix(exprString, "["), "]"), ",")
	for _, element := range elements {
		if !b.Config().ValidBlockAddress(element) {
			return fmt.Errorf("`depends_on` must be a list of block address, invalid address: %s", element)
		}
	}
	return nil
}

func cleanBodyForDecode(hb *hclsyntax.Body) *hclsyntax.Body {
	// Create a new hclsyntax.Body
	newBody := &hclsyntax.Body{
		Attributes: make(hclsyntax.Attributes),
	}

	// Iterate over the attributes of the original body
	for attrName, attr := range hb.Attributes {
		if MetaAttributeNames.Contains(attrName) {
			continue
		}
		newBody.Attributes[attrName] = attr
	}

	for _, nb := range hb.Blocks {
		if MetaNestedBlockNames.Contains(nb.Type) {
			continue
		}
		cloneBody := *nb.Body
		cloneNb := *nb
		cloneNb.Body = cleanBodyForDecode(&cloneBody)
		newBody.Blocks = append(newBody.Blocks, &cloneNb)
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
	ctyValues := Value(b)
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
