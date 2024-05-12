package golden

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"os"
)

var DslFullName string
var DslAbbreviation string

var _ Variable = &VariableBlock{}
var _ PrePlanBlock = &VariableBlock{}
var _ PlanBlock = &VariableBlock{}

type Variable interface {
	SingleValueBlock
	Variable()
}

type VariableBlock struct {
	*BaseBlock
	VariableType  *cty.Type
	VariableValue cty.Value
}

func (v *VariableBlock) ExecuteDuringPlan() error {
	return nil
}

func (v *VariableBlock) Type() string {
	return ""
}

func (v *VariableBlock) BlockType() string {
	return "variable"
}

func (v *VariableBlock) AddressLength() int {
	return 2
}

func (v *VariableBlock) CanExecutePrePlan() bool {
	return true
}

func (v *VariableBlock) Value() cty.Value {
	return v.VariableValue
}

func (v *VariableBlock) Variable() {}

func (v *VariableBlock) ExecuteBeforePlan() error {
	if err := v.ParseVariableType(); err != nil {
		return err
	}
	panic("implement me")
}

func (v *VariableBlock) ParseVariableType() error {
	typeAttr, ok := v.HclBlock().Body.Attributes["type"]
	if !ok {
		return nil
	}
	t, diag := typeexpr.Type(typeAttr.Expr)
	if diag.HasErrors() {
		return diag
	}
	v.VariableType = &t
	return nil
}

func (v *VariableBlock) ReadValueFromEnv() (*cty.Value, error) {
	env := os.Getenv(fmt.Sprintf("%s_VAR_%s", DslAbbreviation, v.name))
	if env == "" {
		return nil, nil
	}
	exp, diag := hclsyntax.ParseExpression([]byte(env), "", hcl.InitialPos)
	if diag.HasErrors() {
		return nil, diag
	}
	value, diag := exp.Value(nil)
	if diag.HasErrors() {
		return nil, diag
	}
	return &value, nil
}
