package p0d

import (
	"fmt"
	"github.com/hako/durafmt"
	. "github.com/logrusorgru/aurora"
	"math"
	"strings"
	"time"
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

func (p *ProgressBar) render(curSecs float64, pod *P0d) string {
	if pod.Stop.IsZero() {
		pctProgress := curSecs / float64(p.maxSecs)
		fs := int(math.Ceil(pctProgress * float64(p.size)))

		b := strings.Builder{}
		b.WriteString("sending requests: ")
		b.WriteString(Yellow(OPEN).String())

		f := strings.Builder{}
		for i := 0; i < fs; i++ {
			if i < fs-1 {
				f.WriteString(FILLED)
			} else {
				f.WriteString(CURRENT)
			}
		}
		b.WriteString(Cyan(f.String()).String())

		for j := fs; j <= p.size; j++ {
			b.WriteString(EMPTY)
		}
		b.WriteString(Yellow(CLOSE).String())
		//remaining whole seconds
		t := (time.Second * time.Duration(p.maxSecs-int(curSecs))).Truncate(time.Second)
		b.WriteString(fmt.Sprintf(" %s", Cyan(durafmt.Parse(t).LimitFirstN(2).String()).String()))
		return b.String()
	} else {
		//truncate runtime as seconds
		elapsed := durafmt.Parse(pod.Stop.Sub(pod.Start).Truncate(time.Second)).LimitFirstN(2).String()
		return fmt.Sprintf("total runtime: %v", Cyan(elapsed))
	}
}
