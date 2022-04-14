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
	v := flag.Bool("v", false, "print the server version")
	flag.Parse()

	if *v {
		mode = Version
	}

	switch mode {
	case Test:
		pod := p0d.NewP0d(*t, *c)
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
	case "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "WARN":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
