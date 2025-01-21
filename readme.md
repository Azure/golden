# Golden

Golden is a project implemented in Go language. It is a DSL (Domain Specific Language) engine that uses HCL (HashiCorp Configuration Language) as its base, like [`grept`](https://github.com/Azure/grept).

Golden assumes a DSL engine which is composited by Plan phase and Apply phase, just like [Terraform](https://www.terraform.io/).

It supports two block interfaces: [`PlanBlock`](./plan_block.go) and [`ApplyBlock`](./apply_block.go), you can implement your own block type, in Terraform, there are `data`, `resource`, `local`, `variable`, `output`. In `grept`, there are `data`, `rule`, `fix`, `local`.

Golden has implemented `local` block.

Golden has implemented support for `for_each` and `precondition` in blocks.

A simple example to show how to customize your own DSL is in our roadmap.
