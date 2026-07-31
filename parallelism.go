package golden

// Parallelism enables parallel DAG execution and returns the maximum number of
// callbacks that may run at the same time. The value must be greater than zero.
type Parallelism interface {
	Parallelism() int
}
