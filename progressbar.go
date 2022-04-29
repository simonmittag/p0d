package p0d

import (
	"fmt"
	. "github.com/logrusorgru/aurora"
	"math"
	"strings"
)

type ProgressBar struct {
	curSecs int
	maxSecs int
	size    int
}

const EMPTY = " "
const FILLED = "="
const CURRENT = ">"
const OPEN = "["
const CLOSE = "]"

func (p *ProgressBar) render(curSecs float64) string {
	pctProgress := curSecs / float64(p.maxSecs)
	fs := int(math.Ceil(pctProgress * float64(p.size)))

	b := strings.Builder{}
	b.WriteString(Yellow(OPEN).String())
	for i := 0; i < fs; i++ {
		if i < fs-1 {
			b.WriteString(Cyan(FILLED).String())
		} else {
			b.WriteString(Cyan(CURRENT).String())
		}
	}
	for j := fs; j <= p.size; j++ {
		b.WriteString(EMPTY)
	}
	b.WriteString(Yellow(CLOSE).String())

	b.WriteString(Cyan(fmt.Sprintf(" [%ds/%ds]", int(curSecs), p.maxSecs-int(curSecs))).String())
	r := b.String()
	return r
}
