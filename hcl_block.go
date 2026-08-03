package golden

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type HclBlock struct {
	*hclsyntax.Block
	wb *hclwrite.Block
	*ForEach
	attributes map[string]*HclAttribute
	blocks     []*HclBlock
}

func NewHclBlock(rb *hclsyntax.Block, wb *hclwrite.Block, each *ForEach) *HclBlock {
	hb := &HclBlock{
		Block:      rb,
		wb:         wb,
		ForEach:    each,
		attributes: make(map[string]*HclAttribute),
	}
	for n, ra := range rb.Body.Attributes {
		hb.attributes[n] = NewHclAttribute(ra, wb.Body().Attributes()[n])
	}
	for i, nrb := range rb.Body.Blocks {
		hb.blocks = append(hb.blocks, NewHclBlock(nrb, wb.Body().Blocks()[i], nil))
	}
	return hb
}

func (hb *HclBlock) Attributes() map[string]*HclAttribute {
	return hb.attributes
}

func (hb *HclBlock) NestedBlocks() []*HclBlock {
	return hb.blocks
}

type ForEach struct {
	key   cty.Value
	value cty.Value
}

func NewForEach(key, value cty.Value) *ForEach {
	return &ForEach{
		key:   key,
		value: value,
	}
}

func AsHclBlocks(syntaxBlocks hclsyntax.Blocks, writeBlocks []*hclwrite.Block) []*HclBlock {
	var blocks []*HclBlock
	for i, b := range syntaxBlocks {
		if b.Type == "locals" {
			blocks = append(blocks, readRawHclLocalBlocks(b, writeBlocks[i])...)
			continue
		}
		var rbs = readRawHclSyntaxBlock(b)
		for _, hb := range rbs {
			blocks = append(blocks, NewHclBlock(hb, writeBlocks[i], nil))
		}
	}
	return blocks
}

func readRawHclLocalBlocks(syntaxBlock *hclsyntax.Block, writeBlock *hclwrite.Block) []*HclBlock {
	syntaxAttributes := syntaxBlock.Body.Attributes
	writeAttributes := writeBlock.Body().Attributes()
	if len(syntaxAttributes) != len(writeAttributes) {
		panic("syntax and write locals blocks contain different numbers of attributes")
	}

	names := make([]string, 0, len(syntaxAttributes))
	for name := range syntaxAttributes {
		names = append(names, name)
	}
	sort.Strings(names)

	blocks := make([]*HclBlock, 0, len(names))
	for _, name := range names {
		syntaxAttribute := syntaxAttributes[name]
		writeAttribute, ok := writeAttributes[name]
		if !ok {
			panic(fmt.Sprintf("write attribute %q is missing from the corresponding locals block", name))
		}

		rb := &hclsyntax.Block{
			Type:   "local",
			Labels: []string{"", name},
			Body: &hclsyntax.Body{
				Attributes: map[string]*hclsyntax.Attribute{
					"value": {
						Name:        "value",
						Expr:        syntaxAttribute.Expr,
						SrcRange:    syntaxAttribute.SrcRange,
						NameRange:   syntaxAttribute.NameRange,
						EqualsRange: syntaxAttribute.EqualsRange,
					},
				},
				SrcRange: syntaxAttribute.NameRange,
				EndRange: syntaxAttribute.SrcRange,
			},
		}
		wb := hclwrite.NewBlock("local", []string{"", name})
		wb.Body().SetAttributeRaw("value", writeAttribute.Expr().BuildTokens(hclwrite.Tokens{}))
		blocks = append(blocks, NewHclBlock(rb, wb, nil))
	}
	return blocks
}

func (hb *HclBlock) ExpandDynamicBlocks(evalContext *hcl.EvalContext) (*HclBlock, error) {
	newHb := &HclBlock{
		Block:      hb.Block,
		wb:         hb.wb,
		ForEach:    hb.ForEach,
		attributes: hb.attributes,
		blocks:     []*HclBlock{},
	}
	var newNestedBlocks []*hclsyntax.Block
	for _, block := range hb.blocks {
		if block.Type != "dynamic" {
			expandedBlock, err := block.ExpandDynamicBlocks(evalContext)
			if err != nil {
				return nil, err
			}
			if err = expandedBlock.evaluateAttributes(evalContext); err != nil {
				return nil, err
			}
			newHb.blocks = append(newHb.blocks, expandedBlock)
			newNestedBlocks = append(newNestedBlocks, expandedBlock.Block)
			continue
		}

		forEachAttr, ok := block.Attributes()["for_each"]
		if !ok {
			return nil, fmt.Errorf("`dynamic` block must have `for_each` attribute")
		}

		forEachValue, diag := forEachAttr.Expr.Value(evalContext)
		if diag.HasErrors() {
			return nil, diag
		}

		if !forEachValue.CanIterateElements() {
			return nil, fmt.Errorf("incorrect type for `for_each`, must be a collection")
		}

		iterator := forEachValue.ElementIterator()
		for iterator.Next() {
			key, value := iterator.Element()
			newContext := evalContext.NewChild()
			newContext.Variables = map[string]cty.Value{
				block.Labels[0]: cty.ObjectVal(map[string]cty.Value{
					"key":   key,
					"value": value,
				}),
			}

			for _, innerBlock := range block.blocks {
				innerBlock = CloneHclBlock(innerBlock)
				if innerBlock.Type != "content" {
					return nil, fmt.Errorf("`dynamic` block should contain `content` block only")
				}

				expandedInnerBlock, err := innerBlock.ExpandDynamicBlocks(newContext)
				if err != nil {
					return nil, err
				}
				expandedInnerBlock.Type = block.Labels[0]
				if err = expandedInnerBlock.evaluateAttributes(newContext); err != nil {
					return nil, err
				}
				newHb.blocks = append(newHb.blocks, expandedInnerBlock)
				newNestedBlocks = append(newNestedBlocks, expandedInnerBlock.Block)
			}
		}
	}
	newHb.Body.Blocks = newNestedBlocks
	return newHb, nil
}

func readRawHclSyntaxBlock(b *hclsyntax.Block) []*hclsyntax.Block {
	switch b.Type {
	case "variable":
		{
			return []*hclsyntax.Block{
				{
					Type:            "variable",
					Labels:          append([]string{""}, b.Labels...),
					Body:            b.Body,
					TypeRange:       b.TypeRange,
					LabelRanges:     b.LabelRanges,
					OpenBraceRange:  b.OpenBraceRange,
					CloseBraceRange: b.CloseBraceRange,
				},
			}
		}
	default:
		if block, ok := blockSamples[b.Type]; ok && block.Type() == "" {
			b = &hclsyntax.Block{
				Type:            b.Type,
				Labels:          append([]string{""}, b.Labels...),
				Body:            b.Body,
				TypeRange:       b.TypeRange,
				LabelRanges:     b.LabelRanges,
				OpenBraceRange:  b.OpenBraceRange,
				CloseBraceRange: b.CloseBraceRange,
			}
		}

		return []*hclsyntax.Block{b}
	}
}

func clone[T any](v *T) *T {
	c := *v
	return &c
}
func CloneHclBlock(hb *HclBlock) *HclBlock {
	// Clone the hclsyntax.Block
	cloneBlock := CloneHclSyntaxBlock(hb.Block)

	// Clone the HclBlock
	cloneHb := &HclBlock{
		Block:      cloneBlock,
		wb:         clone(hb.wb),
		ForEach:    hb.ForEach,
		attributes: make(map[string]*HclAttribute),
		blocks:     make([]*HclBlock, len(hb.blocks)),
	}

	// Clone attributes
	for name, attr := range hb.attributes {
		cloneHb.attributes[name] = clone(attr)
	}

	// Clone blocks recursively
	for i, block := range hb.blocks {
		cloneHb.blocks[i] = CloneHclBlock(block)
	}

	return cloneHb
}

func CloneHclSyntaxBlock(hb *hclsyntax.Block) *hclsyntax.Block {
	// Clone the block itself
	cloneBlock := clone(hb)

	// Clone the body
	cloneBody := &hclsyntax.Body{
		Attributes: make(hclsyntax.Attributes),
		Blocks:     make(hclsyntax.Blocks, len(hb.Body.Blocks)),
	}

	// Clone attributes
	for name, attr := range hb.Body.Attributes {
		cloneBody.Attributes[name] = clone(attr)
	}

	// Clone blocks recursively
	for i, block := range hb.Body.Blocks {
		cloneBody.Blocks[i] = CloneHclSyntaxBlock(block)
	}

	// Assign the cloned body to the cloned block
	cloneBlock.Body = cloneBody

	return cloneBlock
}

func (hb *HclBlock) evaluateAttributes(ctx *hcl.EvalContext) error {
	for attributeName, attribute := range hb.Body.Attributes {
		v, diag := attribute.Expr.Value(ctx)
		if diag.HasErrors() {
			return diag
		}
		hb.Body.Attributes[attributeName] = &hclsyntax.Attribute{
			Name: attributeName,
			Expr: &hclsyntax.LiteralValueExpr{
				Val: v,
			},
			SrcRange: attribute.SrcRange,
		}
	}
	return nil
}
