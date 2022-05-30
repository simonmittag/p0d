package p0d

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewP0dFromFile(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")
	if p.Config.Res.Code != 200 {
		t.Error("incorrect response code")
	}
	if p.Config.Exec.Concurrency != 128 {
		t.Error("incorrect concurrency")
	}
	if p.Config.Exec.DurationSeconds != 30 {
		t.Error("incorrect duration seconds")
	}
	if p.Config.Exec.DialTimeoutSeconds != 3 {
		t.Error("incorrect dialtimeout seconds")
	}
	if p.Config.Req.Method != "GET" {
		t.Error("incorrect method")
	}
	if p.Config.Req.Url != "http://localhost:60083/mse6/get" {
		t.Error("incorrect url")
	}
	if p.Config.Req.Headers[0]["Accept-Encoding"] != "identity" {
		t.Error("incorrect header")
	}
	if p.ID == "" {
		t.Error("incorrect ID")
	}
}

func TestNewP0dWithValues(t *testing.T) {
	p := NewP0dWithValues(7, 6, "http://localhost/", "1.1", "", true)

	if p.Config.Res.Code != 200 {
		t.Error("incorrect response code")
	}
	if p.Config.Exec.Concurrency != 7 {
		t.Error("incorrect concurrency")
	}
	if p.Config.Exec.DurationSeconds != 6 {
		t.Error("incorrect duration seconds")
	}
	if p.Config.Exec.DialTimeoutSeconds != 3 {
		t.Error("incorrect dialtimeout seconds")
	}
	if p.Config.Exec.HttpVersion != 1.1 {
		t.Error("incorrect http version")
	}
	if p.Config.Req.Method != "GET" {
		t.Error("incorrect method")
	}
	if p.Config.Req.Url != "http://localhost/" {
		t.Error("incorrect url")
	}
	if len(p.Config.Req.Headers) > 0 {
		t.Error("incorrect header, shouldn't be specified")
	}
	if p.ID == "" {
		t.Error("incorrect ID")
	}

}

func TestLogBootstrap(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")
	p.initLog()
}

func TestLogSummary(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")
	p.logSummary()
}

func TestDoReqAtmpt(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123456789")
	}))
	defer svr.Close()

	//we hack the config's URL to point at our mock server so we can execute the test
	p.Config.Req.Url = svr.URL

	//nonblocking channel we now use done to signal
	ras := make(chan ReqAtmpt, 65535)
	done := make(chan struct{})
	//fire this off in goroutine
	go p.doReqAtmpts(0, ras, done)

	//then wait for signal from completed reqAtmpt.
	ra := <-ras

	//we tell ReqAtmpt to shut down.
	done <- struct{}{}
	close(done)

	if ra.ResCode != 200 {
		t.Error("should have returned response code 200")
	}
	if ra.ResBytes != 125 {
		t.Error("should have returned 125 Bytes")
	}
	if ra.ResErr != "" {
		t.Error("should not have errored")
	}
}

func TestRace(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123456789")
	}))
	defer svr.Close()

	//we hack the config's URL to point at our mock server so we can execute the test
	p.Config.Req.Url = svr.URL
	p.Config.Exec.DurationSeconds = 1
	p.Race()

}

func TestRaceWithOutput(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "testoutput.json")

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123456789")
	}))
	defer svr.Close()

	//we hack the config's URL to point at our mock server so we can execute the test
	p.Config.Req.Url = svr.URL

	//test this only shortly
	p.Config.Exec.DurationSeconds = 1
	p.Race()

	//for good measure
	os.Remove(p.Output)
}

func TestTimerPhase(t *testing.T) {
	p := P0d{Time: Time{Phase: Bootstrap}}

	p.setTimerPhase(Bootstrap)
	if !p.isTimerPhase(Bootstrap) {
		t.Error("should have bootstrap")
	}

	p.setTimerPhase(RampUp)
	if p.isTimerPhase(Bootstrap) {
		t.Error("should NOT have bootstrap")
	}
	if !p.isTimerPhase(RampUp) {
		t.Error("should have rampup")
	}

	p.setTimerPhase(RampDown)
	if p.isTimerPhase(Bootstrap) {
		t.Error("should NOT have bootstrap")
	}
	if p.isTimerPhase(RampUp) {
		t.Error("should NOT have rampup")
	}
	if !p.isTimerPhase(RampDown) {
		t.Error("should have rampdown")
	}

	p.setTimerPhase(Draining)
	if p.isTimerPhase(Bootstrap) {
		t.Error("should NOT have bootstrap")
	}
	if p.isTimerPhase(RampUp) {
		t.Error("should NOT have rampup")
	}
	if p.isTimerPhase(RampDown) {
		t.Error("should NOT have rampdown")
	}
	if !p.isTimerPhase(Draining) {
		t.Error("should have draining")
	}

	p.setTimerPhase(Drained)
	if p.isTimerPhase(Bootstrap) {
		t.Error("should NOT have bootstrap")
	}
	if p.isTimerPhase(RampUp) {
		t.Error("should NOT have rampup")
	}
	if p.isTimerPhase(RampDown) {
		t.Error("should NOT have rampdown")
	}
	if p.isTimerPhase(Draining) {
		t.Error("should NOT have draining")
	}
	if !p.isTimerPhase(Drained) {
		t.Error("should have drained")
	}

	p.setTimerPhase(Done)
	if p.isTimerPhase(Bootstrap) {
		t.Error("should NOT have bootstrap")
	}
	if p.isTimerPhase(RampUp) {
		t.Error("should NOT have rampup")
	}
	if p.isTimerPhase(RampDown) {
		t.Error("should NOT have rampdown")
	}
	if p.isTimerPhase(Draining) {
		t.Error("should NOT have draining")
	}
	if p.isTimerPhase(Drained) {
		t.Error("should NOT have drained")
	}
	if !p.isTimerPhase(Done) {
		t.Error("should have done")
	}

	p2 := P0d{Time: Time{Phase: RampUp}}
	p2.setTimerPhase(Done)
	if p2.isTimerPhase(RampUp) {
		t.Error("should NOT have rampup")
	}
	if p2.isTimerPhase(RampDown) {
		t.Error("should NOT have rampdown")
	}
	if !p2.isTimerPhase(Done) {
		t.Error("should have done")
	}
}

func TestStaggerThreadsDuration(t *testing.T) {
	p := P0d{
		Config: Config{
			Exec: Exec{
				RampSeconds: 12,
				Concurrency: 3,
			},
		},
	}

	f := p.staggerThreadsDuration()
	if f != 4.0*time.Second {
		t.Error("invalid stagger period")
	}

	p2 := P0d{
		Config: Config{
			Exec: Exec{
				RampSeconds: 60,
				Concurrency: 2048,
			},
		},
	}

	f2 := p2.staggerThreadsDuration()
	if f2 != time.Duration(float64(0.029296875)*float64(time.Second)) {
		t.Error("invalid stagger period")
	}
}
