package golden

import "github.com/zclconf/go-cty/cty"

var NoValue = VariableValueRead{}

type VariableValueRead struct {
	Name  string
	Value *cty.Value
	Error error
}

func NewVariableValueRead(name string, value *cty.Value, err error) VariableValueRead {
	return VariableValueRead{
		Name:  name,
		Value: value,
		Error: err,
	}
}

func (r VariableValueRead) HasError() bool {
	return r.Error == nil
}
