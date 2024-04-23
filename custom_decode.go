package golden

import "github.com/hashicorp/hcl/v2"

type CustomDecode interface {
	Decode(*HclBlock, *hcl.EvalContext) error
}
