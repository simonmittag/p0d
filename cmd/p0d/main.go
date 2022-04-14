package main

import (
	"flag"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/simonmittag/p0d"
	"os"
	"strings"
)

type Mode uint8

const (
	Test Mode = 1 << iota
	Version
)

var pattern = "/mse6/"

func main() {
	initLogger()
	mode := Test
	t := flag.Int("t", 1, "amount of parallel execution threads")
	c := flag.Int("c", 1, "maximum amount of parallel TCP connections used")
	d := flag.Int("d", 10, "time in seconds to run p0d")
	u := flag.String("u", "", "url to use")
	v := flag.Bool("v", false, "print the server version")
	flag.Parse()

	if *v {
		mode = Version
	}

	switch mode {
	case Test:
		pod := p0d.NewP0d(*t, *c, *d, *u)
		pod.Race()
	case Version:
		printVersion()
	}
}

func printVersion() {
	fmt.Printf("p0d %s\n", p0d.Version)
	os.Exit(0)
}

func initLogger() {
	logLevel := strings.ToUpper(os.Getenv("LOGLEVEL"))
	switch logLevel {
	case "DEBUG":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "WARN":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
