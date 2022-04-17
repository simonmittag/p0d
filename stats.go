package p0d

import (
	"math"
	"time"
)

type Stats struct {
	Start                    time.Time
	Elpsd                    time.Duration
	ReqAtmpts                int
	ReqAtmptsSec             int
	SumBytes                 int64
	MeanBytesSec             int
	SumElpsdAtmptLatency     time.Duration
	MeanElpsdAtmptLatency    time.Duration
	SumMatchingResponseCodes int
	PctMatchingResponseCodes float32
	SumErrors                int
	PctErrors                float32
	ErrorTypes               map[string]int
}

func (s *Stats) update(atmpt ReqAtmpt, now time.Time, cfg Config) {
	s.ReqAtmpts++
	s.Elpsd = now.Sub(s.Start)
	s.ReqAtmptsSec = int(math.Floor(float64(s.ReqAtmpts) / s.Elpsd.Seconds()))

	s.SumBytes += atmpt.ResponseBytes
	s.MeanBytesSec = int(math.Floor(float64(s.SumBytes) / s.Elpsd.Seconds()))
	s.SumElpsdAtmptLatency += atmpt.Elapsed
	s.MeanElpsdAtmptLatency = s.SumElpsdAtmptLatency / time.Duration(s.ReqAtmpts)

	if atmpt.ResponseCode == cfg.Res.Code {
		s.SumMatchingResponseCodes++
	}
	s.PctMatchingResponseCodes = 100 * (float32(s.SumMatchingResponseCodes) / float32(s.ReqAtmpts))

	if atmpt.ResponseError != "" {
		s.SumErrors++
		s.ErrorTypes[atmpt.ResponseError]++
	}
	s.PctErrors = 100 * (float32(s.SumErrors) / float32(s.ReqAtmpts))
}
