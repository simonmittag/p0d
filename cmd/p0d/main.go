package main

import (
	"flag"
	"fmt"
	"github.com/simonmittag/p0d"
)

type Mode uint8

const (
	Test Mode = 1 << iota
	File
	Version
)

var pattern = "/mse6/"

func main() {
	mode := Test
	C := flag.String("C", "", "load configuration from yml file")
	O := flag.String("O", "", "save detailed JSON output to file")
	t := flag.Int("t", 1, "amount of parallel execution threads")
	c := flag.Int("c", 1, "maximum amount of parallel TCP connections used")
	d := flag.Int("d", 10, "time in seconds to run p0d")
	u := flag.String("u", "", "url to use")
	h := flag.String("h", "1.1", "http version to use. Values are 1.1 and 2 (which works only with "+
		"TLS URLs). Defaults to 1.1")
	v := flag.Bool("v", false, "print p0d version")
	flag.Parse()

	if len(*C) > 0 {
		mode = File
	}

	if *v {
		mode = Version
	}

	switch mode {
	case Test:
		pod := p0d.NewP0dWithValues(*t, *c, *d, *u, *h, *O)
		pod.Race()
	case File:
		pod := p0d.NewP0dFromFile(*C, *O)
		pod.Race()
	case Version:
		printVersion()
	}
}

func printVersion() {
	fmt.Printf("p0d %s\n", p0d.Version)
}
