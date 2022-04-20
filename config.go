package p0d

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/rs/zerolog/log"
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
	HttpVersion        string
}

const UNLIMITED int = -1

var httpVers map[string]string = map[string]string{"1.1": "1.1", "2": "2"}

func loadConfigFromFile(fileName string) *Config {
	log.Debug().Msgf("loading config from file '%s'", fileName)
	cfgPanic := func(err error) {
		if err != nil {
			msg := fmt.Sprintf("unable to load config from %s, exiting...", fileName)
			log.Fatal().Msg(msg)
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
		cfg.Exec.Connections = UNLIMITED
	}
	if cfg.Exec.DurationSeconds == 0 {
		cfg.Exec.DurationSeconds = 10
	}
	if cfg.Exec.DialTimeoutSeconds == 0 {
		cfg.Exec.DialTimeoutSeconds = 3
	}
	if cfg.Exec.HttpVersion == "" {
		cfg.Exec.HttpVersion = "1.1"
	} else {
		if _, ok := httpVers[cfg.Exec.HttpVersion]; !ok {
			msg := fmt.Sprintf("bad http version %s, must be one of [1.1, 2], exiting...", cfg.Exec.HttpVersion)
			log.Fatal().Msg(msg)
			panic(msg)
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
		msg := "request url not specified, exiting..."
		log.Fatal().Msg(msg)
		panic(msg)
	}
	return cfg
}

func (cfg Config) scaffoldHttpClient() *http.Client {
	t := &http.Transport{
		DisableCompression: true,
		DialContext: (&net.Dialer{
			//we are aborting after n seconds of dial connect to complete and treat the dial as degraded
			Timeout: time.Duration(cfg.Exec.DialTimeoutSeconds) * time.Second,
		}).DialContext,
		//TLS handshake timeout is the same as connection timeout
		TLSHandshakeTimeout: 3,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},

		MaxConnsPerHost:     cfg.Exec.Connections,
		MaxIdleConns:        cfg.Exec.Connections,
		MaxIdleConnsPerHost: cfg.Exec.Connections,
		IdleConnTimeout:     3 * time.Second,
	}

	//see https://stackoverflow.com/questions/57683132/turning-off-connection-pool-for-go-http-client
	if cfg.Exec.Connections == UNLIMITED {
		log.Debug().Msg("transport connection pool disabled")
		t.DisableKeepAlives = true
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
