package p0d

import (
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"os"
)

type Config struct {
	Req  Req
	Res  Res
	Exec Exec
}

type Req struct {
	Method  string
	Url     string
	Headers map[string]string
	Body    string
}

type Res struct {
	Code string
}

type Exec struct {
	DurationSeconds int
	Threads         int
	Connections     int
}

func loadConfigFromFile(fileName string) *Config {
	log.Debug().Msgf("attempting config from file '%s'", fileName)
	f, err := os.Open(fileName)
	defer f.Close()
	if err != nil {
		msg := fmt.Sprintf("unable to load config from %s, exiting...", fileName)
		log.Fatal().Msg(msg)
		panic(msg)
	}
	yml, _ := ioutil.ReadAll(f)
	jsn, _ := yaml.YAMLToJSON(yml)

	c := &Config{}
	json.Unmarshal(jsn, c)
	return c
}

const UNLIMITED int = -1

func (cfg *Config) validate() *Config {
	if cfg.Exec.Connections == 0 {
		cfg.Exec.Connections = UNLIMITED
	}
	if cfg.Exec.DurationSeconds == 0 {
		cfg.Exec.DurationSeconds = 10
	}
	if cfg.Req.Method == "" {
		cfg.Req.Method = "GET"
	}
	if cfg.Res.Code == "" {
		cfg.Res.Code = "200"
	}
	if cfg.Req.Url == "" {
		msg := "no req URL, panicking"
		log.Fatal().Msg(msg)
		panic(msg)
	}
	return cfg
}
