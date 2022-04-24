package p0d

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewP0dFromFile(t *testing.T) {
	p := NewP0dFromFile("./config_get.yml", "")
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
	p := NewP0dFromFile("./config_get.yml", "")
	p.logBootstrap()
}

func TestLogSummary(t *testing.T) {
	p := NewP0dFromFile("./config_get.yml", "")
	p.logSummary("1 minute")
}

func TestDoReqAtmpt(t *testing.T) {
	p := NewP0dFromFile("./config_get.yml", "")

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123456789")
	}))
	defer svr.Close()

	//we hack the config's URL to point at our mock server so we can execute the test
	p.Config.Req.Url = svr.URL

	//blocking channel but we don't want to run crazy inside doReqAtmtp, need only 1 response
	ras := make(chan ReqAtmpt)

	//fire this off in goroutine
	go p.doReqAtmpt(ras)

	//then wait for signal from completed reqAtmpt.
	ra := <-ras
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
	p := NewP0dFromFile("./config_get.yml", "")

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123456789")
	}))
	defer svr.Close()

	//we hack the config's URL to point at our mock server so we can execute the test
	p.Config.Req.Url = svr.URL
	p.Config.Exec.DurationSeconds = 3
	p.Race()

}
