package golden

import (
	"fmt"
	"sync"
)

var valuePromoter variableValuePromoter = stdVariableValuePromoter{}
var promoterMutex = &sync.Mutex{}

type variableValuePromoter interface {
	printf(format string, a ...any) (n int, err error)
	scanln(a ...any) (n int, err error)
}

type stdVariableValuePromoter struct{}

func (s stdVariableValuePromoter) printf(format string, a ...any) (n int, err error) {
	return fmt.Printf(format, a...)
}

func (s stdVariableValuePromoter) scanln(a ...any) (n int, err error) {
	return fmt.Scanln(a...)
}
