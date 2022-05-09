package p0d

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewP0dFromFile(t *testing.T) {
	p := NewP0dFromFile("./examples/config_get.yml", "")
	if p.Config.Res.Code != 200 {
		t.Error("incorrect response code")
	}
	if p.Config.Exec.Connections != 128 {
		t.Error("incorrect connections")
	}
	if p.Config.Exec.DurationSeconds != 30 {
		t.Error("incorrect duration seconds")
	}
	if p.Config.Exec.DialTimeoutSeconds != 3 {
		t.Error("incorrect dialtimeout seconds")
	}
	if p.Config.Exec.Threads != 128 {
		t.Error("incorrect threads")
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
	p := NewP0dWithValues(8, 7, 6, "http://localhost/", "1.1", "")

	if p.Config.Res.Code != 200 {
		t.Error("incorrect response code")
	}
	if p.Config.Exec.Connections != 7 {
		t.Error("incorrect connections")
	}
	if p.Config.Exec.DurationSeconds != 6 {
		t.Error("incorrect duration seconds")
	}
	if p.Config.Exec.DialTimeoutSeconds != 3 {
		t.Error("incorrect dialtimeout seconds")
	}
	if p.Config.Exec.Threads != 8 {
		t.Error("incorrect threads")
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
	p.logBootstrap()
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
	go p.doReqAtmpt(ras, done)

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
