package p0d

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	. "github.com/logrusorgru/aurora"
	"golang.org/x/net/http2"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	Req  Req
	Res  Res
	Exec Exec
	File string
}

type Req struct {
	Method        string
	Url           string
	Headers       []map[string]string
	ContentType   string
	Body          string
	FormData      []map[string]string
	FormDataFiles map[string][]byte
}

type Res struct {
	Code int
}

type Exec struct {
	Mode               string
	DurationSeconds    int
	Threads            int
	Connections        int
	DialTimeoutSeconds int64
	LogSampling        float32
	SpacingMillis      int64
	HttpVersion        float32
}

const UNLIMITED int = -1

const http11 = 1.1
const http20 = 2

var httpVers = map[float32]float32{http11: http11, http20: http20}

func loadConfigFromFile(fileName string) *Config {
	cfgPanic := func(err error) {
		if err != nil {
			msg := Red(fmt.Sprintf("unable to load config from '%s', exiting...", fileName))
			logv(msg)
			os.Exit(-1)
		}
	}

	f, err := os.Open(fileName)
	defer f.Close()
	cfgPanic(err)

	yml, _ := ioutil.ReadAll(f)
	jsn, _ := yaml.YAMLToJSON(yml)

	c := &Config{}
	err = json.Unmarshal(jsn, c)
	cfgPanic(err)
	return c
}

func (cfg *Config) validate() *Config {
	//we always want this.
	cfg.Req.Method = strings.ToUpper(cfg.Req.Method)
	if cfg.Req.Method == "" {
		cfg.Req.Method = "GET"
	}

	if cfg.Exec.Threads == 0 {
		cfg.Exec.Threads = 1
	}
	if cfg.Exec.Connections == 0 {
		cfg.Exec.Connections = 1
	}
	if cfg.Exec.DurationSeconds == 0 {
		cfg.Exec.DurationSeconds = 10
	}
	if cfg.Exec.DialTimeoutSeconds == 0 {
		cfg.Exec.DialTimeoutSeconds = 3
	}
	if cfg.Exec.Mode == "" {
		cfg.Exec.Mode = "decimal"
	}
	if cfg.Exec.HttpVersion == 0 {
		cfg.Exec.HttpVersion = http11
	} else {
		if _, ok := httpVers[cfg.Exec.HttpVersion]; !ok {
			hv := fmt.Sprintf("%.1f", cfg.Exec.HttpVersion)
			cfg.panic(fmt.Sprintf("bad http version %s, must be one of [1.1, 2.0], exiting...", hv))
		}
	}
	if cfg.Exec.LogSampling <= 0 || cfg.Exec.LogSampling > 1 {
		//default to all
		cfg.Exec.LogSampling = 1
	}
	if cfg.Exec.SpacingMillis < 0 {
		//default to all
		cfg.Exec.SpacingMillis = 0
	}

	cfg.validateReqBody()

	if cfg.Req.Url == "" {
		cfg.panic("request url not specified")
	}
	if cfg.Res.Code == 0 {
		cfg.Res.Code = 200
	}
	return cfg
}

func (cfg *Config) validateReqBody() {
	if len(cfg.Req.Body) > 0 {
		if len(cfg.Req.FormData) > 0 {
			cfg.panic("when specifying request body, cannot have form data")
		}
	}

	if len(cfg.Req.FormData) > 0 {
		if cfg.Req.Method != "POST" {
			cfg.panic("when specifying form data, method must be POST")
		}
		if len(cfg.Req.Body) > 0 {
			cfg.panic("when specifying form data, cannot specify body, use formData param")
		}
		cfg.setFormDataContentType()

		cfg.Req.FormDataFiles = make(map[string][]byte, 0)
		for i, fd := range cfg.Req.FormData {
			for k, v := range fd {
				if strings.HasPrefix(k, "@") {
					dat, err := os.ReadFile(v)
					if err != nil {
						cfg.panic(fmt.Sprintf("unable to read file: %s", v))
					}
					cfg.Req.FormDataFiles[k] = dat
					f, _ := os.Open(v)
					fs, _ := f.Stat()
					cfg.Req.FormData[i] = map[string]string{k: fs.Name()}
					dat = nil
				}
			}
		}
	} else if contains(bodyTypes, cfg.Req.Method) {
		cfg.setDefaultPostContentType()
	}
}

func (cfg *Config) setDefaultPostContentType() {
	jsonc := "application/json"
	jsonct := map[string]string{"Content-Type": jsonc}
	matched := false
	for _, h := range cfg.Req.Headers {
		for k, v := range h {
			if k == "Content-Type" {
				matched = true
				cfg.Req.ContentType = v
			}
		}
	}
	//we only set default content type if it wasn't specified otherwise.
	if !matched {
		cfg.Req.Headers = append(cfg.Req.Headers, jsonct)
		cfg.Req.ContentType = jsonc
	}
}

func (cfg *Config) setFormDataContentType() {
	const contentType = "Content-Type"

	if len(cfg.Req.Headers) > 0 {
		//defaults to urlencoded
		form := "application/x-www-form-urlencoded"
		formct := map[string]string{contentType: form}

		matched := false
		for _, h := range cfg.Req.Headers {
			for k, v := range h {
				if k == contentType {
					matched = true
					cfg.Req.ContentType = v
				}
			}
		}
		if !matched {
			cfg.Req.Headers = append(cfg.Req.Headers, formct)
			cfg.Req.ContentType = form
		}
	}
}

func (cfg Config) scaffoldHttpClient() *http.Client {
	tlsc := &tls.Config{
		MinVersion:         tls.VersionTLS11,
		MaxVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	t := &http.Transport{
		DisableCompression: true,
		DialContext: (&net.Dialer{
			//we are aborting after n seconds of dial connect to complete and treat the dial as degraded
			Timeout: time.Duration(cfg.Exec.DialTimeoutSeconds) * time.Second,
		}).DialContext,
		//TLS handshake timeout is the same as connection timeout
		TLSHandshakeTimeout: time.Duration(cfg.Exec.DialTimeoutSeconds) * time.Second,
		TLSClientConfig:     tlsc,
		MaxConnsPerHost:     cfg.Exec.Connections,
		MaxIdleConns:        cfg.Exec.Connections,
		MaxIdleConnsPerHost: cfg.Exec.Connections,
		IdleConnTimeout:     time.Duration(cfg.Exec.DialTimeoutSeconds) * time.Second,
		TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	//see https://stackoverflow.com/questions/57683132/turning-off-connection-pool-for-go-http-client
	if cfg.Exec.Connections == UNLIMITED {
		t.DisableKeepAlives = true
		logv(Yellow("transport connection pool disabled for http/1.1"))
	}

	if cfg.Exec.HttpVersion == http20 {
		http2.ConfigureTransport(t)
	}

	return &http.Client{
		Transport: t,
	}
}

func (cfg Config) byteCount(b int64) string {
	switch strings.TrimSpace(cfg.Exec.Mode) {
	case "binary":
		return ByteCountIEC(b)
	default:
		return ByteCountSI(b)
	}
}

func (cfg Config) panic(msg string) {
	logv(Red(msg))
	os.Exit(-1)
}
