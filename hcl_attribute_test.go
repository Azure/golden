package golden_test

import (
	"github.com/Azure/golden"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/lonegunmanb/hclfuncs"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
	"testing"
)

func TestHclAttribute_ExprString(t *testing.T) {
	tests := []struct {
		name     string
		attrExpr string
		want     string
	}{
		{
			name:     "simple string attribute",
			attrExpr: `"Hello, World!"`,
			want:     `"Hello, World!"`,
		},
		{
			name:     "integer attribute",
			attrExpr: `42`,
			want:     `42`,
		},
		{
			name:     "float attribute",
			attrExpr: `3.14`,
			want:     `3.14`,
		},
		{
			name:     "boolean attribute",
			attrExpr: `true`,
			want:     `true`,
		},
		{
			name:     "exp",
			attrExpr: `1+2`,
			want:     `1+2`,
		},
		{
			name:     "function",
			attrExpr: `max(5, 12, 9)`,
			want:     `max(5, 12, 9)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hclConfig := `attr = ` + tt.attrExpr + ` `

			// Parse the HCL configuration using hclsyntax.ParseConfig
			syntaxFile, diag := hclsyntax.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
			if diag.HasErrors() {
				t.Fatalf("Failed to parse HCL: %s", diag.Error())
			}

			// Parse the HCL configuration using hclwrite.ParseConfig
			writeFile, diag := hclwrite.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
			if diag.HasErrors() {
				t.Fatalf("Failed to parse HCL: %s", diag.Error())
			}

			// Get the first attribute from the parsed HCL configuration
			syntaxAttr := syntaxFile.Body.(*hclsyntax.Body).Attributes["attr"]
			writeAttr := writeFile.Body().Attributes()["attr"]

			// Call NewHclAttribute
			hclAttr := golden.NewHclAttribute(syntaxAttr, writeAttr)

			// Verify that the ExprString method returns the correct string
			assert.Equal(t, tt.want, hclAttr.ExprString())
		})
	}
}

func TestHclAttribute_Value(t *testing.T) {
	tests := []struct {
		name        string
		attrExpr    string
		want        cty.Value
		expectError bool
	}{
		{
			name:     "simple string attribute",
			attrExpr: `"Hello, World!"`,
			want:     cty.StringVal("Hello, World!"),
		},
		{
			name:     "integer attribute",
			attrExpr: `42`,
			want:     cty.NumberIntVal(42),
		},
		{
			name:     "float attribute",
			attrExpr: `3.14`,
			want:     cty.NumberFloatVal(3.14),
		},
		{
			name:     "boolean attribute",
			attrExpr: `true`,
			want:     cty.BoolVal(true),
		},
		{
			name:     "exp",
			attrExpr: `1+2`,
			want:     cty.NumberIntVal(3),
		},
		{
			name:     "function",
			attrExpr: `max(5, 12, 9)`,
			want:     cty.NumberIntVal(12),
		},
		{
			name:     "variable, could be found in ctx",
			attrExpr: `var.number`,
			want:     cty.NumberIntVal(42),
		},
		{
			name:        "variable, could not be found in ctx",
			attrExpr:    "var.not_exist",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hclConfig := `attr = ` + tt.attrExpr + ` `

			// Parse the HCL configuration using hclsyntax.ParseConfig
			syntaxFile, diag := hclsyntax.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
			if diag.HasErrors() {
				t.Fatalf("Failed to parse HCL: %s", diag.Error())
			}

			// Parse the HCL configuration using hclwrite.ParseConfig
			writeFile, diag := hclwrite.ParseConfig([]byte(hclConfig), "test", hcl.InitialPos)
			if diag.HasErrors() {
				t.Fatalf("Failed to parse HCL: %s", diag.Error())
			}

			// Get the first attribute from the parsed HCL configuration
			syntaxAttr := syntaxFile.Body.(*hclsyntax.Body).Attributes["attr"]
			writeAttr := writeFile.Body().Attributes()["attr"]

			// Call NewHclAttribute
			hclAttr := golden.NewHclAttribute(syntaxAttr, writeAttr)

			ctx := &hcl.EvalContext{
				Functions: hclfuncs.Functions("."),
				Variables: map[string]cty.Value{
					"var": cty.ObjectVal(map[string]cty.Value{
						"number": cty.NumberIntVal(42),
					}),
				},
			}

			// Verify that the ExprString method returns the correct string
			value, err := hclAttr.Value(ctx)
			if tt.expectError {
				assert.NotNil(t, err)
			} else {
				switch tt.want.Type() {
				case cty.Number:
					wanted, _ := tt.want.AsBigFloat().Float32()
					actual, _ := value.AsBigFloat().Float32()
					assert.Equal(t, wanted, actual)
				case cty.Bool:
					assert.Equal(t, tt.want.True(), value.True())
				case cty.String:
					assert.Equal(t, tt.want.AsString(), value.AsString())
				}
			}
		})
	}
}
