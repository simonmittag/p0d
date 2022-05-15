package p0d

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	. "github.com/logrusorgru/aurora"
	"golang.org/x/net/http2"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	RampSeconds        int
	Concurrency        int
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

	if cfg.Exec.Concurrency == 0 {
		cfg.Exec.Concurrency = 1
	}
	if cfg.Exec.DurationSeconds == 0 {
		cfg.Exec.DurationSeconds = 10
	} else {
		if cfg.Exec.DurationSeconds < 3 {
			cfg.panic("duration cannot be less than 3 seconds")
		}
	}

	if cfg.Exec.RampSeconds == 0 {
		cfg.Exec.RampSeconds = int(math.Ceil(float64(cfg.Exec.DurationSeconds) / 10))
	} else {
		if float64(cfg.Exec.RampSeconds) > (float64(cfg.Exec.DurationSeconds) / 2) {
			cfg.panic("ramp time cannot be longer than half the duration")
		}
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
	} else {
		u, e := url.Parse(cfg.Req.Url)
		if e != nil {
			cfg.panic(e.Error())
		}
		_, p, _ := net.SplitHostPort(u.Host)
		if len(p) > 0 {
			p1, e2 := strconv.Atoi(p)
			if e2 != nil {
				cfg.panic(e.Error())
			}
			if p1 < 0 || p1 > 65535 {
				cfg.panic(fmt.Sprintf("valid port range is [0-65535], yours: %d", p1))
			}
		}
	}

	if cfg.Res.Code == 0 {
		cfg.Res.Code = 200
	}
	return cfg
}

func (cfg *Config) getRemotePort() uint16 {
	u, _ := url.Parse(cfg.Req.Url)
	_, p, _ := net.SplitHostPort(u.Host)
	if p == N {
		if u.Scheme == "http" {
			p = "80"
		}
		if u.Scheme == "https" {
			p = "443"
		}
	}
	p1, _ := strconv.Atoi(p)
	return uint16(p1)
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
		cfg.setDefaultFormDataContentType()

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
	cfg.setContentType("application/json", false)
}

func (cfg *Config) setDefaultFormDataContentType() {
	cfg.setContentType("application/x-www-form-urlencoded", false)
}

func (cfg *Config) setContentType(contentType string, overwrite bool) {
	const ctkey = "Content-Type"
	ctobj := map[string]string{ctkey: contentType}

	if len(cfg.Req.Headers) > 0 {
		matched := false
		for i, h := range cfg.Req.Headers {
			for k, v := range h {
				if k == ctkey {
					matched = true
					if overwrite {
						cfg.Req.Headers[i] = ctobj
						cfg.Req.ContentType = contentType
					} else {
						cfg.Req.Headers[i][ct] = v
						cfg.Req.ContentType = v
					}
				}
			}
		}
		if !matched {
			cfg.Req.Headers = append(cfg.Req.Headers, ctobj)
			cfg.Req.ContentType = contentType
		}
	} else {
		cfg.Req.Headers = append(cfg.Req.Headers, ctobj)
		cfg.Req.ContentType = contentType
	}
}

func (cfg *Config) hasContentType(contentType string) bool {
	for _, h := range cfg.Req.Headers {
		for k, v := range h {
			if k == "Content-Type" {
				if v != contentType {
					return false
				}
			}
		}
	}
	if cfg.Req.ContentType != contentType {
		return false
	}
	return true
}

func (cfg Config) scaffoldHttpClients() map[int]*http.Client {
	cs := make(map[int]*http.Client, cfg.Exec.Concurrency)

	//to bypass connection re-use inside the pool for multiple parallel requests using streams in http/2
	//we create multiple clients, limited to one connection, whereas for http/1.1 it is one large pool that is shared.
	if cfg.Exec.HttpVersion == http20 {
		for i := 0; i < cfg.Exec.Concurrency; i++ {
			c := cfg.scaffoldHttpClient(1)
			cs[i] = c
		}
	} else {
		c := cfg.scaffoldHttpClient(cfg.Exec.Concurrency)
		for i := 0; i < cfg.Exec.Concurrency; i++ {
			cs[i] = c
		}
	}
	return cs
}

const httpIdleTimeout = time.Duration(1) * time.Second

func (cfg Config) scaffoldHttpClient(max int) *http.Client {
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
		MaxConnsPerHost:     max,
		MaxIdleConns:        max,
		MaxIdleConnsPerHost: max,
		IdleConnTimeout:     httpIdleTimeout,
		TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	//see https://stackoverflow.com/questions/57683132/turning-off-connection-pool-for-go-http-client
	if cfg.Exec.Concurrency == UNLIMITED {
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
