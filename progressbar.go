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
const RAMP = "-"
const FULL = "="
const CURRENT = ">"
const OPEN = "["
const CLOSE = "]"
const rt = "runtime: "
const trt = "total runtime: %v"

func (p *ProgressBar) updateRampStateForTimerPhase(now time.Time, pod *P0d) {
	if pod.isTimerPhase(RampUp) ||
		pod.isTimerPhase(RampDown) ||
		//fixes UI bug during Bootstrap phase
		pod.isTimerPhase(Bootstrap) {
		p.markRamp(now, pod)
	}
}

func (p *ProgressBar) markRamp(chunkTime time.Time, pod *P0d) {
	i := p.chunkPropIndexFor(chunkTime, pod)
	p.chunkProps[i].isRamp = p.chunkProps[i].isRamp || true
}

func (p *ProgressBar) markError(chunkTime time.Time, pod *P0d) {
	i := p.chunkPropIndexFor(chunkTime, pod)
	p.chunkProps[i].hasErrors = p.chunkProps[i].hasErrors || true
}

func (p *ProgressBar) chunkPropIndexFor(chunkTime time.Time, pod *P0d) int {
	chunkSizeSeconds := float64(pod.Config.Exec.DurationSeconds) / float64(p.size)
	elapsed := chunkTime.Sub(pod.Start).Seconds()
	if elapsed > 0 {
		i := int(math.Floor(elapsed / chunkSizeSeconds))
		if i <= p.size-1 {
			return i
		} else {
			return p.size - 1
		}
	} else {
		return 0
	}
}

func (p *ProgressBar) render(now time.Time, pod *P0d) string {

	if pod.Stop.IsZero() {
		fsi := p.chunkPropIndexFor(now, pod)

		b := strings.Builder{}
		b.WriteString(rt)
		b.WriteString(Yellow(OPEN).String())

		f := strings.Builder{}
		for i := 0; i <= fsi; i++ {
			if i < fsi {
				if p.chunkProps[i].isRamp == true {
					if p.chunkProps[i].hasErrors {
						f.WriteString(Red(RAMP).String())
					} else {
						f.WriteString(Cyan(RAMP).String())
					}
				} else {
					if p.chunkProps[i].hasErrors {
						f.WriteString(Red(FULL).String())
					} else {
						f.WriteString(Cyan(FULL).String())
					}
				}
			} else {
				if p.chunkProps[i].hasErrors {
					f.WriteString(Red(CURRENT).String())
				} else {
					f.WriteString(Cyan(CURRENT).String())
				}
			}
		}
		b.WriteString(f.String())

		for j := fsi; j < p.size-1; j++ {
			b.WriteString(EMPTY)
		}
		b.WriteString(Yellow(CLOSE).String())
		//remaining whole seconds
		curSecs := now.Sub(pod.Start).Seconds()
		t := (time.Second * time.Duration(p.maxSecs-int(curSecs))).Truncate(time.Second)
		b.WriteString(fmt.Sprintf("%s", Cyan(" eta ").String()+Cyan(durafmt.Parse(t).LimitFirstN(2).String()).String()))
		return b.String()
	} else {
		//truncate runtime as seconds
		elapsed := durafmt.Parse(pod.Stop.Sub(pod.Start).Truncate(time.Second)).LimitFirstN(2).String()

		return fmt.Sprintf(trt, Cyan(elapsed))
	}
}
