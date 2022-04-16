package p0d

import (
	"errors"
	"testing"
	"time"
)

func TestUpdateStats(t *testing.T) {
	cfg := Config{Res: Res{Code: 200}}

	s := Stats{
		Start: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	g := ReqAtmpt{
		Start:         time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC),
		Stop:          time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC),
		Elapsed:       time.Duration(2 * time.Second),
		ResponseCode:  200,
		ResponseBytes: 1000,
		ResponseError: nil,
	}

	now := time.Date(2000, 1, 1, 0, 0, 4, 0, time.UTC)
	s.update(g, now, cfg)

	if s.ReqAtmpts != 1 {
		t.Error("request attempts incorrect")
	}
	if s.SumBytes != 1000 {
		t.Error("sumbytes incorrect")
	}
	if s.MeanBytesSec != 250 {
		t.Error("mean bytes per sec incorrect")
	}
	if s.Elpsd != 4*time.Second {
		t.Error("status elapsed time incorrect")
	}
	if s.SumElpsdAtmpt != 2*time.Second {
		t.Error("sum of elapsed time for attempts incorrect")
	}
	if s.MeanElpsdAtmpt != 2*time.Second {
		t.Error("mean of elapsed time for attempts incorrect")
	}
	if s.SumMatchingResponseCodes != 1 {
		t.Error("sum matching response codes incorrect")
	}
	if s.PctMatchingResponseCodes != 100.00 {
		t.Error("percent matching response codes incorrect")
	}
	if s.SumErrors > 0 || s.PctErrors > 0 {
		t.Error("should not have errors")
	}

	now = time.Date(2000, 1, 1, 0, 0, 6, 0, time.UTC)

	//we update the stats now with more data, this time 1s req but it's 6s down the timeline
	//this request has an error
	g2 := ReqAtmpt{
		Start:         time.Date(2000, 1, 1, 0, 0, 5, 0, time.UTC),
		Stop:          time.Date(2000, 1, 1, 0, 0, 6, 0, time.UTC),
		Elapsed:       time.Duration(1 * time.Second),
		ResponseCode:  201,
		ResponseBytes: 1000,
		ResponseError: errors.New("i'm so bad"),
	}

	s.update(g2, now, cfg)

	if s.ReqAtmpts != 2 {
		t.Error("request attempts incorrect")
	}
	if s.SumBytes != 2000 {
		t.Error("sumbytes incorrect")
	}
	if s.MeanBytesSec != 333 {
		t.Error("mean bytes per sec incorrect")
	}
	if s.Elpsd != 6*time.Second {
		t.Error("status elapsed time incorrect")
	}
	if s.SumElpsdAtmpt != 3*time.Second {
		t.Error("sum of elapsed time for attempts incorrect")
	}
	if s.MeanElpsdAtmpt != 1500*time.Millisecond {
		t.Error("mean of elapsed time for attempts incorrect")
	}
	if s.SumMatchingResponseCodes != 1 {
		t.Error("sum matching response codes incorrect")
	}
	if s.PctMatchingResponseCodes != 50.00 {
		t.Error("percent matching response codes incorrect")
	}
	if s.SumErrors != 1 {
		t.Error("should have errors")
	}
	if s.PctErrors != 50.00 {
		t.Error("should have errors")
	}
}
