package golden

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

func validateForEachValueAvailability(value cty.Value, sourceRange hcl.Range) hcl.Diagnostics {
	if value.IsNull() {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid for_each value",
			Detail:   "The for_each expression is null. Use an empty map ({}) or an empty set (toset([])) when no instances should be created.",
			Subject:  sourceRange.Ptr(),
		}}
	}
	if !value.IsKnown() {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid for_each value",
			Detail:   "The for_each expression is unknown, so Golden cannot determine which instance keys to create before expansion. Use a map whose keys are known before planning; unknown results can be placed in the map values.",
			Subject:  sourceRange.Ptr(),
		}}
	}
	if value.Type().IsSetType() && !value.IsWhollyKnown() {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid for_each value",
			Detail:   "The for_each set contains values that are unknown before expansion. Set elements become instance keys, so use a map with statically known keys and place unknown results in the map values.",
			Subject:  sourceRange.Ptr(),
		}}
	}
	return nil
}
