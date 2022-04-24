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
}

type Req struct {
	Method  string
	Url     string
	Headers []map[string]string
	Body    string
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
	log("loading config from file '%s'", Yellow(fileName))
	cfgPanic := func(err error) {
		if err != nil {
			msg := Red(fmt.Sprintf("unable to load config from %s, exiting...", fileName))
			logv(msg)
			panic(msg)
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
	if cfg.Exec.Connections == 0 {
		cfg.Exec.Connections = 16
	}
	if cfg.Exec.DurationSeconds == 0 {
		cfg.Exec.DurationSeconds = 10
	}
	if cfg.Exec.DialTimeoutSeconds == 0 {
		cfg.Exec.DialTimeoutSeconds = 3
	}
	if cfg.Exec.HttpVersion == 0 {
		cfg.Exec.HttpVersion = http11
	} else {
		if _, ok := httpVers[cfg.Exec.HttpVersion]; !ok {
			cfg.panic(Red(fmt.Sprintf("bad http version %s, must be one of [1.1, 2.0], exiting...", fmt.Sprintf("%.1f", cfg.Exec.HttpVersion))))
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
	if cfg.Req.Method == "" {
		cfg.Req.Method = "GET"
	}
	if cfg.Res.Code == 0 {
		cfg.Res.Code = 200
	}
	if cfg.Req.Url == "" {
		cfg.panic(Red("request url not specified, exiting..."))
	}
	return cfg
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

func (cfg Config) panic(msg Value) {
	logv(msg)
	panic(msg)
}
