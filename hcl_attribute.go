package golden

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"strings"
)

type HclAttribute struct {
	*hclsyntax.Attribute
	wa *hclwrite.Attribute
}

func NewHclAttribute(ra *hclsyntax.Attribute, wa *hclwrite.Attribute) *HclAttribute {
	return &HclAttribute{
		Attribute: ra,
		wa:        wa,
	}
}

func (ha *HclAttribute) ExprTokens() hclwrite.Tokens {
	return ha.wa.Expr().BuildTokens(hclwrite.Tokens{})
}

func (ha *HclAttribute) ExprString() string {
	tokens := ha.wa.Expr().BuildTokens(hclwrite.Tokens{})
	return strings.TrimSpace(string(tokens.Bytes()))
}

func (ha *HclAttribute) Value(ctx *hcl.EvalContext) (cty.Value, error) {
	value, diag := ha.Expr.Value(ctx)
	if diag.HasErrors() {
		return cty.Value{}, diag
	}
	return value, nil
}
