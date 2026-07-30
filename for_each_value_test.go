package golden

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func (s *forEachTestSuite) TestForEachNullValueReturnsDiagnostic() {
	config := `
variable "items" {
  type    = map(string)
  default = null
}

data "dummy" "sample" {
  for_each = var.items
}
`
	s.dummyFsWithFiles(map[string]string{"test.hcl": config})

	var err error
	s.NotPanics(func() {
		_, err = BuildDummyConfig("", "", nil, nil)
	})
	s.Error(err)
	s.ErrorContains(err, "Invalid for_each value")
	s.ErrorContains(err, "The for_each expression is null")
	s.ErrorContains(err, "Use an empty map ({}) or an empty set (toset([]))")
	s.ErrorContains(err, "test.hcl")
}

func TestValidateForEachValueAvailability(t *testing.T) {
	unknownString := cty.UnknownVal(cty.String)
	sourceRange := hcl.Range{
		Filename: "test.hcl",
		Start:    hcl.Pos{Line: 2, Column: 14, Byte: 30},
		End:      hcl.Pos{Line: 2, Column: 23, Byte: 39},
	}
	tests := []struct {
		name           string
		value          cty.Value
		expectedDetail string
	}{
		{name: "null map", value: cty.NullVal(cty.Map(cty.String)), expectedDetail: "is null"},
		{name: "null set", value: cty.NullVal(cty.Set(cty.String)), expectedDetail: "is null"},
		{name: "unknown map", value: cty.UnknownVal(cty.Map(cty.String)), expectedDetail: "expression is unknown"},
		{name: "unknown set", value: cty.UnknownVal(cty.Set(cty.String)), expectedDetail: "expression is unknown"},
		{name: "set with unknown element", value: cty.SetVal([]cty.Value{unknownString}), expectedDetail: "set contains values that are unknown"},
		{name: "map with known key and unknown value", value: cty.MapVal(map[string]cty.Value{"alpha": unknownString})},
		{name: "object with known key and unknown value", value: cty.ObjectVal(map[string]cty.Value{"alpha": unknownString})},
		{name: "list with known index and unknown value", value: cty.ListVal([]cty.Value{unknownString})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diags := validateForEachValueAvailability(test.value, sourceRange)
			if test.expectedDetail == "" {
				require.False(t, diags.HasErrors())
				return
			}
			require.True(t, diags.HasErrors())
			require.Len(t, diags, 1)
			require.Contains(t, diags[0].Detail, test.expectedDetail)
			require.Equal(t, sourceRange, *diags[0].Subject)
		})
	}
}
