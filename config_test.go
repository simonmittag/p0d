package p0d

import (
	"testing"
)

func TestEmptyConfigValidate(t *testing.T) {
	cfg := Config{
		Req: Req{
			Url: "http://localhost:8080/blah",
		},
	}
	got := cfg.validate()
	if got.Res.Code != 200 {
		t.Error("invalid default res code")
	}
	if got.Req.Method != "GET" {
		t.Error("invalid default req method")
	}
	if got.Exec.Mode != "decimal" {
		t.Error("invalid default exec mode")
	}
	if got.Exec.Mode != "decimal" {
		t.Error("invalid default exec mode")
	}
	if got.Exec.HttpVersion != 1.1 {
		t.Error("invalid default http version")
	}
	if got.Exec.DialTimeoutSeconds != 3 {
		t.Error("invalid default dial timeout")
	}
	if got.Exec.DurationSeconds != 10 {
		t.Error("invalid default duration seconds")
	}
	if got.Exec.LogSampling != 1 {
		t.Error("invalid default logsampling")
	}
	if got.Exec.SpacingMillis != 0 {
		t.Error("invalid default spacing millis")
	}
	if got.Exec.Threads != 1 {
		t.Error("invalid default threads")
	}
	if got.Exec.Connections != 1 {
		t.Error("invalid default connections")
	}
	if got.Exec.Connections != 1 {
		t.Error("invalid default connections")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	cfg := loadConfigFromFile("./examples/config_get.yml")

	//we already tested the other scenarios elsewhere.
	if cfg.Res.Code != 200 {
		t.Error("incorrectly parsed")
	}

	cfg.validate()

	if cfg.Req.Url == "" {
		t.Error("URL not parsed, checks that validate ran.")
	}
}

func TestFormConfigValidate(t *testing.T) {
	testForApplicationXWWWFormUrlEncoded := func(cfg *Config) {
		for _, h := range cfg.Req.Headers {
			for k, v := range h {
				if k == "Content-Type" {
					if v != "application/x-www-form-urlencoded" {
						t.Errorf("Content-Type should be application/x-www-form-urlencoded, was %v", v)
					}
				}
			}
		}
	}

	cfg := loadConfigFromFile("./examples/config_formpost.yml")
	cfg.validate()

	//test form header present
	testForApplicationXWWWFormUrlEncoded(cfg)

	//wipe the form header
	cfg.Req.Headers = make([]map[string]string, 0)

	//revalidate and test the header is back it should be inserted when form data is found.
	cfg.validate()
	testForApplicationXWWWFormUrlEncoded(cfg)

}

func TestPostConfigValidate(t *testing.T) {
	testForApplicationJson := func(cfg *Config) {
		for _, h := range cfg.Req.Headers {
			for k, v := range h {
				if k == "Content-Type" {
					if v != "application/json" {
						t.Errorf("Content-Type should be application/json, was %v", v)
					}
				}
			}
		}
	}

	cfg := loadConfigFromFile("./examples/config_post.yml")
	cfg.validate()

	//test form header present
	testForApplicationJson(cfg)

	//wipe the form header
	cfg.Req.Headers = make([]map[string]string, 0)

	//revalidate and test the header is back it should be inserted when not specified.
	cfg.validate()
	testForApplicationJson(cfg)

}

func TestScaffoldHTTPClient(t *testing.T) {
	cfg := loadConfigFromFile("./examples/config_get.yml")

	h := cfg.scaffoldHttpClient()
	if h.Transport == nil {
		t.Error("http client not configured")
	}
}

func TestByteCount(t *testing.T) {
	cfg := loadConfigFromFile("./examples/config_get.yml")

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

func TestSetDefaultFormDataContentType(t *testing.T) {
	cfg := Config{}
	cfg.setContentType(multipartFormdata, false)

	if !cfg.hasContentType(multipartFormdata) {
		t.Errorf("should be multipart/form-data")
	}

	//should not overwrite
	cfg.setContentType(applicationJson, false)

	if !cfg.hasContentType(multipartFormdata) {
		t.Errorf("should be multipart/form-data")
	}

	//now it should
	cfg.setContentType(applicationJson, true)

	if !cfg.hasContentType(applicationJson) {
		t.Errorf("should be application/json")
	}
}
