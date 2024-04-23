package golden

import "github.com/hashicorp/hcl/v2"

type BaseDecode interface {
	BaseDecode(hb *HclBlock, context *hcl.EvalContext) error
}
