package p0d

import "testing"

func TestNewP0dFromFile(t *testing.T) {
	p := NewP0dFromFile("./config_get.yml")
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
	p := NewP0dWithValues(8, 7, 6, "http://localhost/")

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
