package main

import (
	"flag"
	"fmt"
	"github.com/simonmittag/p0d"
	"os"
)

type Mode uint8

const (
	Cli Mode = 1 << iota
	File
	Usage
	Version
)

var pattern = "/mse6/"

func main() {
	var mode Mode

	C := flag.String("C", "", "load configuration from yml file")
	O := flag.String("O", "", "save detailed output to json file")
	c := flag.Int("c", 1, "maximum amount of concurrent TCP connections used")
	d := flag.Int("d", 10, "time in seconds to run p0d")
	H := flag.String("H", "1.1", "http version to use. Values are 1.1 and 2 (which works only with "+
		"TLS URLs). Defaults to 1.1")
	s := flag.Bool("s", false, "skip internet speed test i.e. for local targets")
	h := flag.Bool("h", false, "print usage instructions")
	v := flag.Bool("v", false, "print version")
	var u string

	flag.Parse()

	if *v {
		mode = Version
	} else if *h || (flag.NFlag() == 0 && len(flag.Args()) == 0) {
		mode = Usage
	} else if len(*C) > 0 {
		mode = File
	} else {
		mode = Cli
		u = flag.Arg(0)
	}

	var pod *p0d.P0d
	switch mode {
	case Cli:
		pod = p0d.NewP0dWithValues(*c, *d, u, *H, *O, *s)
		pod.Race()
	case File:
		pod = p0d.NewP0dFromFile(*C, *O)
		pod.Race()
	case Usage:
		printUsage()
	case Version:
		printVersion()
	}

	if mode == Cli || mode == File {
		if pod == nil || pod.ReqStats.SumErrors > 0 || pod.Interrupted {
			os.Exit(-1)
		}
	}
}

func printVersion() {
	p0d.PrintVersion()
}

func printUsage() {
	p0d.PrintLogo()
	p0d.PrintVersion()
	fmt.Print("\nusage: p0d [-f flag] [URL]\n\n flags:\n")
	flag.PrintDefaults()
}
