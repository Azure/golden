package golden

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"os"
	"strings"
)

var _ Variable = &VariableBlock{}
var _ PrePlanBlock = &VariableBlock{}
var _ PlanBlock = &VariableBlock{}
var _ CustomDecode = &VariableBlock{}
var _ BlockCustomizedRefType = &VariableBlock{}

type Variable interface {
	SingleValueBlock
	Variable()
}

type VariableValidation struct {
	Condition    bool   `hcl:"condition"`
	ErrorMessage string `hcl:"error_message"`
}

type VariableBlock struct {
	*BaseBlock
	Description   *string
	Validations   []VariableValidation
	variableType  *cty.Type
	variableValue *cty.Value
}

func (v *VariableBlock) Decode(block *HclBlock, context *hcl.EvalContext) error {
	return nil
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

func (v *VariableBlock) CustomizedRefType() string {
	return "var"
}

func (v *VariableBlock) Address() string {
	return fmt.Sprintf("var.%s", v.Name())
}

func (v *VariableBlock) AddressLength() int {
	return 2
}

func (v *VariableBlock) CanExecutePrePlan() bool {
	return true
}

func (v *VariableBlock) Value() cty.Value {
	if v.variableValue == nil {
		return cty.NilVal
	}
	return *v.variableValue
}

func (v *VariableBlock) Variable() {}

func (v *VariableBlock) ExecuteBeforePlan() error {
	err := v.parseDescription()
	if err != nil {
		return err
	}
	if err = v.parseVariableType(); err != nil {
		return err
	}
	variableRead, err := v.readValue()
	if err != nil {
		return err
	}
	if variableRead.Error != nil {
		return variableRead.Error
	}
	value := variableRead.Value
	if value == nil {
		return fmt.Errorf("cannot evaluate value for var.%s", v.Name())
	}
	if v.variableType != nil && value.Type() != *v.variableType {
		convertedValue, err := convert.Convert(*value, *v.variableType)
		if err != nil {
			return fmt.Errorf("incompatible type for var.%s, want %s, got %s", v.Name(), v.variableType.GoString(), value.Type().GoString())
		}
		value = &convertedValue
	}
	v.variableValue = value
	return v.validationCheck()
}

func (v *VariableBlock) parseVariableType() error {
	typeAttr, ok := v.HclBlock().Body.Attributes["type"]
	if !ok {
		v.variableType = nil
		return nil
	}
	t, diag := typeexpr.Type(typeAttr.Expr)
	if diag.HasErrors() {
		return diag
	}
	v.variableType = &t
	return nil
}

func (v *VariableBlock) readValue() (VariableValueRead, error) {
	variables, err := v.c.readInputVariables()
	if err != nil {
		return NoValue, err
	}
	read, ok := variables[v.Name()]
	if ok && read != NoValue {
		return read, nil
	}
	defaultRead := v.readDefaultValue()
	if defaultRead != NoValue {
		return defaultRead, nil
	}
	return v.readFromPromote()
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

func (v *VariableBlock) readFromPromote() (VariableValueRead, error) {
	promoterMutex.Lock()
	defer promoterMutex.Unlock()
	_, _ = valuePromoter.printf("var.%s\n", v.Name())
	if v.Description != nil {
		_, _ = valuePromoter.printf("  %s\n\n", *v.Description)
	}
	_, _ = valuePromoter.printf("  Enter a value: ")
	var in string
	_, err := valuePromoter.scanln(&in)
	if err != nil {
		return NoValue, err
	}
	_, _ = valuePromoter.printf("\n")
	return v.parseVariableValueFromString(in, false), nil
}

func (v *VariableBlock) parseDescription() error {
	attr, ok := v.HclBlock().Attributes()["description"]
	if !ok {
		return nil
	}
	value, diag := attr.Expr.Value(nil)
	if diag.HasErrors() {
		return diag
	}
	if value.Type() != cty.String {
		return fmt.Errorf("incorrect type for `description` %s, got %s, want %s", attr.Range().String(), value.Type().GoString(), cty.String.GoString())
	}
	desc := value.AsString()
	v.Description = &desc
	return nil
}

func (v *VariableBlock) validationCheck() error {
	var err error
	for _, nb := range v.HclBlock().NestedBlocks() {
		if nb.Type != "validation" {
			continue
		}
		ctx := v.c.EmptyEvalContext()
		ctx.Variables = map[string]cty.Value{
			"var": cty.ObjectVal(map[string]cty.Value{
				v.Name(): *v.variableValue,
			}),
		}
		var vb VariableValidation
		diag := gohcl.DecodeBody(nb.Body, ctx, &vb)
		if diag.HasErrors() {
			err = multierror.Append(err, diag)
			continue
		}
		v.Validations = append(v.Validations, vb)
	}
	if err != nil {
		return err
	}
	for _, validation := range v.Validations {
		if validation.Condition {
			continue
		}
		err = multierror.Append(err, fmt.Errorf("invalid value for variable %s\n%s\n%s", v.Name(), v.HclBlock().Range().String(), validation.ErrorMessage))
	}
	return err
}
