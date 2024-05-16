package golden

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"os"
	"strings"
)

var _ Variable = &VariableBlock{}
var _ PrePlanBlock = &VariableBlock{}
var _ PlanBlock = &VariableBlock{}

type Variable interface {
	SingleValueBlock
	Variable()
}

type VariableBlock struct {
	*BaseBlock
	Description   *string `hcl:"description"`
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
	if err := v.parseVariableType(); err != nil {
		return err
	}
	return nil
}

func (v *VariableBlock) parseVariableType() error {
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

func (v *VariableBlock) readValue() VariableValueRead {
	variables, _ := v.BaseBlock.c.readInputVariables()
	read, ok := variables[v.Name()]
	if ok && read != NoValue {
		return read
	}
	defaultRead := v.readDefaultValue()
	return defaultRead
	//if defaultRead != NoValue {
	//	return defaultRead
	//}
	//return v.readFromPromote()
}

func (v *VariableBlock) readValueFromEnv() VariableValueRead {
	env := os.Getenv(fmt.Sprintf("%s_VAR_%s", strings.ToUpper(v.c.DslAbbreviation()), v.name))
	return v.parseVariableValueFromString(env, true)
}

func (v *VariableBlock) readDefaultValue() VariableValueRead {
	defaultAttr, hasDefault := v.HclBlock().Body.Attributes["default"]
	if !hasDefault {
		return NoValue
	}
	value, diag := defaultAttr.Expr.Value(nil)
	if diag.HasErrors() {
		return NewVariableValueRead(v.Name(), nil, diag)
	}
	return NewVariableValueRead(v.Name(), &value, nil)
}

func (v *VariableBlock) parseVariableValueFromString(rawValue string, treatEmptyAsNoValue bool) VariableValueRead {
	if rawValue == "" && treatEmptyAsNoValue {
		return NoValue
	}
	for {
		exp, diag := hclsyntax.ParseExpression([]byte(rawValue), "", hcl.InitialPos)
		if diag.HasErrors() {
			return NewVariableValueRead(v.Name(), nil, diag)
		}
		value, diag := exp.Value(nil)
		if strings.Contains(diag.Error(), "Variables not allowed") {
			rawValue = fmt.Sprintf(`"%s"`, rawValue)
			continue
		}
		if diag.HasErrors() {
			return NewVariableValueRead(v.Name(), nil, diag)
		}
		return NewVariableValueRead(v.Name(), &value, nil)
	}
}
