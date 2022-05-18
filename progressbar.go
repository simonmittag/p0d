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
	curSecs    int
	maxSecs    int
	size       int
	chunkProps []ChunkProps
}

type ChunkProps struct {
	index     int
	isRamp    bool
	hasErrors bool
}

const EMPTY = " "
const FILLED = "="
const CURRENT = ">"
const OPEN = "["
const CLOSE = "]"
const rt = "runtime: "
const trt = "total runtime: %v"

func (p *ProgressBar) markRamp(chunkTime time.Time, pod *P0d) {
	i := p.chunkPropIndexFor(chunkTime, pod)
	p.chunkProps[i].isRamp = p.chunkProps[i].isRamp || true
}

func (p *ProgressBar) markError(chunkTime time.Time, pod *P0d) {
	i := p.chunkPropIndexFor(chunkTime, pod)
	p.chunkProps[i].hasErrors = p.chunkProps[i].hasErrors || true
}

func (p *ProgressBar) chunkPropIndexFor(chunkTime time.Time, pod *P0d) int {
	chunkSizeSeconds := float64(pod.Stop.Sub(pod.Start).Seconds()) / float64(p.size)
	elapsed := chunkTime.Sub(pod.Start).Seconds()
	return int(math.Floor(elapsed / chunkSizeSeconds))
}

func (p *ProgressBar) render(curSecs float64, pod *P0d) string {
	if pod.Stop.IsZero() {
		pctProgress := curSecs / float64(p.maxSecs)
		fs := int(math.Ceil(pctProgress * float64(p.size)))

		b := strings.Builder{}
		b.WriteString(rt)
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
		b.WriteString(fmt.Sprintf("%s", Cyan(" eta ").String()+Cyan(durafmt.Parse(t).LimitFirstN(2).String()).String()))
		return b.String()
	} else {
		//truncate runtime as seconds
		elapsed := durafmt.Parse(pod.Stop.Sub(pod.Start).Truncate(time.Second)).LimitFirstN(2).String()

		return fmt.Sprintf(trt, Cyan(elapsed))
	}
}
