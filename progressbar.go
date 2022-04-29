package p0d

import (
	. "github.com/logrusorgru/aurora"
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
	fs := 0
	es := p.size

	b := strings.Builder{}
	b.WriteString(Yellow(OPEN).String())
	for i := 0; i < fs; i++ {
		b.WriteString(Cyan(FILLED).String())
		b.WriteString(Cyan(CURRENT).String())
	}
	for j := fs; j < es; j++ {
		b.WriteString(EMPTY)
	}
	b.WriteString(Yellow(CLOSE).String())
	return b.String()
}
