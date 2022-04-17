package p0d

import "testing"

func TestLoadConfigFromFile(t *testing.T) {
	cfg := loadConfigFromFile("./config_get.yml")

	//we already tested the other scenarios elsewhere.
	if cfg.Res.Code != 200 {
		t.Error("incorrectly parsed")
	}

	cfg.validate()

	if cfg.Req.Url == "" {
		t.Error("URL not parsed, checks that validate ran.")
	}
}

func TestScaffoldHTTPClient(t *testing.T) {
	cfg := loadConfigFromFile("./config_get.yml")

	h := cfg.scaffoldHttpClient()
	if h.Transport == nil {
		t.Error("http client not configured")
	}
}

func TestByteCount(t *testing.T) {
	cfg := loadConfigFromFile("./config_get.yml")

	cfg.Exec.Mode = "decimal"

	bc := cfg.byteCount(1000000)
	if bc != "1.0MB" {
		t.Error("incorrect byte count in decimal mode")
	}

	cfg.Exec.Mode = "binary"

	bc = cfg.byteCount(1000000)
	if bc != "976.6KiB" {
		t.Error("incorrect byte count in binary mode")
	}

	cfg.Exec.Mode = "binary"

	bc2 := cfg.byteCount(1048576)
	if bc2 != "1.0MiB" {
		t.Error("incorrect byte count in binary mode")
	}

	cfg.Exec.Mode = "decimal"
	bc2 = cfg.byteCount(1048576)
	if bc2 != "1.0MB" {
		t.Error("incorrect byte count in binary mode")
	}
}
