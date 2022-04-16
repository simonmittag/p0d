package p0d

import (
	"math"
	"sync"
	"time"
)

type Stats struct {
	Start                    time.Time
	Elpsd                    time.Duration
	ReqAtmpts                int
	SumBytes                 int64
	MeanBytesSec             int
	SumElpsdAtmpt            time.Duration
	MeanElpsdAtmpt           time.Duration
	SumMatchingResponseCodes int
	PctMatchingResponseCodes float32
	SumErrors                int
	PctErrors                float32
}

var statsLock = sync.Mutex{}

func (s *Stats) update(atmpt ReqAtmpt, now time.Time, cfg Config) {
	//statsLock.Lock()

	s.ReqAtmpts++
	s.Elpsd = now.Sub(s.Start)
	s.SumBytes += atmpt.ResponseBytes
	s.MeanBytesSec = int(math.Floor(float64(s.SumBytes) / s.Elpsd.Seconds()))
	s.SumElpsdAtmpt += atmpt.Elapsed
	s.MeanElpsdAtmpt = s.SumElpsdAtmpt / time.Duration(s.ReqAtmpts)

	if atmpt.ResponseCode == cfg.Res.Code {
		s.SumMatchingResponseCodes++
	}
	s.PctMatchingResponseCodes = 100 * float32(s.SumMatchingResponseCodes) / float32(s.ReqAtmpts)

	if atmpt.ResponseError != nil {
		s.SumErrors++
	}
	s.PctErrors = 100 * float32(s.SumErrors) / float32(s.ReqAtmpts)

	//statsLock.Unlock()
}
