package golden

import (
	"fmt"
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
		var rbs = readRawHclSyntaxBlock(b)
		var wbs = readRawHclWriteBlock(writeBlocks[i])
		for i, hb := range rbs {
			blocks = append(blocks, NewHclBlock(hb, wbs[i], nil))
		}
	}
	return blocks
}

func (hb *HclBlock) ExpandDynamicBlocks(evalContext *hcl.EvalContext) (*HclBlock, error) {
	return CloneHclBlock(hb).expandDynamicBlocks(evalContext)
}

func (hb *HclBlock) expandDynamicBlocks(evalContext *hcl.EvalContext) (*HclBlock, error) {
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
			expandedBlock, err := block.expandDynamicBlocks(evalContext)
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

				expandedInnerBlock, err := innerBlock.expandDynamicBlocks(newContext)
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
	case "locals":
		{
			var newBlocks []*hclsyntax.Block
			for _, attr := range b.Body.Attributes {
				newBlocks = append(newBlocks, &hclsyntax.Block{
					Type:   "local",
					Labels: []string{"", attr.Name},
					Body: &hclsyntax.Body{
						Attributes: map[string]*hclsyntax.Attribute{
							"value": {
								Name:        "value",
								Expr:        attr.Expr,
								SrcRange:    attr.SrcRange,
								NameRange:   attr.NameRange,
								EqualsRange: attr.EqualsRange,
							},
						},
						SrcRange: attr.NameRange,
						EndRange: attr.SrcRange,
					},
				})
			}
			return newBlocks
		}
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

func readRawHclWriteBlock(b *hclwrite.Block) []*hclwrite.Block {
	if b.Type() != "locals" {
		return []*hclwrite.Block{b}
	}
	var newBlocks []*hclwrite.Block
	for n, attr := range b.Body().Attributes() {
		nb := hclwrite.NewBlock("local", []string{"", n})
		nb.Body().SetAttributeRaw("value", attr.Expr().BuildTokens(hclwrite.Tokens{}))
		newBlocks = append(newBlocks, nb)
	}
	return newBlocks
}

func clone[T any](v *T) *T {
	c := *v
	return &c
}
func CloneHclBlock(hb *HclBlock) *HclBlock {
	cloneBlock := CloneHclSyntaxBlock(hb.Block)
	return cloneHclBlock(hb, cloneBlock)
}

func cloneHclBlock(hb *HclBlock, cloneBlock *hclsyntax.Block) *HclBlock {
	// hclwrite blocks are shared because Golden uses them only to read expression tokens.
	cloneHb := &HclBlock{
		Block:      cloneBlock,
		wb:         hb.wb,
		attributes: make(map[string]*HclAttribute, len(cloneBlock.Body.Attributes)),
		blocks:     make([]*HclBlock, len(hb.blocks)),
	}
	if hb.ForEach != nil {
		cloneHb.ForEach = clone(hb.ForEach)
	}

	for name, attr := range cloneBlock.Body.Attributes {
		var writeAttribute *hclwrite.Attribute
		if sourceAttribute, ok := hb.attributes[name]; ok {
			writeAttribute = sourceAttribute.wa
		}
		cloneHb.attributes[name] = NewHclAttribute(attr, writeAttribute)
	}

	for i, block := range hb.blocks {
		cloneHb.blocks[i] = cloneHclBlock(block, cloneBlock.Body.Blocks[i])
	}

	return cloneHb
}

func CloneHclSyntaxBlock(hb *hclsyntax.Block) *hclsyntax.Block {
	cloneBlock := clone(hb)
	cloneBlock.Labels = append([]string(nil), hb.Labels...)
	cloneBlock.LabelRanges = append([]hcl.Range(nil), hb.LabelRanges...)

	cloneBody := clone(hb.Body)
	cloneBody.Attributes = make(hclsyntax.Attributes, len(hb.Body.Attributes))
	cloneBody.Blocks = make(hclsyntax.Blocks, len(hb.Body.Blocks))

	for name, attr := range hb.Body.Attributes {
		// Expressions are immutable after parsing, so clones can safely share them.
		cloneBody.Attributes[name] = clone(attr)
	}

	for i, block := range hb.Body.Blocks {
		cloneBody.Blocks[i] = CloneHclSyntaxBlock(block)
	}

	cloneBlock.Body = cloneBody
	return cloneBlock
}

func (hb *HclBlock) evaluateAttributes(ctx *hcl.EvalContext) error {
	for attributeName, attribute := range hb.Body.Attributes {
		v, diag := attribute.Expr.Value(ctx)
		if diag.HasErrors() {
			return diag
		}
		evaluatedAttribute := &hclsyntax.Attribute{
			Name: attributeName,
			Expr: &hclsyntax.LiteralValueExpr{
				Val: v,
			},
			SrcRange:    attribute.SrcRange,
			NameRange:   attribute.NameRange,
			EqualsRange: attribute.EqualsRange,
		}
		hb.Body.Attributes[attributeName] = evaluatedAttribute
		if wrappedAttribute, ok := hb.attributes[attributeName]; ok {
			hb.attributes[attributeName] = &HclAttribute{
				Attribute: evaluatedAttribute,
				wa:        wrappedAttribute.wa,
			}
		}
	}
	return nil
}
