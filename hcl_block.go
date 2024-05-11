package golden

import (
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
		hb.blocks = append(hb.blocks, NewHclBlock(nrb, wb.Body().Blocks()[i] /*TODO: dynamic support in the future*/, nil))
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
