package main

import (
	"flag"
	"fmt"
	. "github.com/logrusorgru/aurora"
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
	O := flag.String("O", "", "save detailed JSON output to file")
	t := flag.Int("t", 1, "amount of parallel execution threads")
	c := flag.Int("c", 1, "maximum amount of parallel TCP connections used")
	d := flag.Int("d", 10, "time in seconds to run p0d")
	H := flag.String("H", "1.1", "http version to use. Values are 1.1 and 2 (which works only with "+
		"TLS URLs). Defaults to 1.1")
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
		pod = p0d.NewP0dWithValues(*t, *c, *d, u, *H, *O)
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
		if pod == nil || pod.Stats.SumErrors > 0 || pod.Interrupted {
			os.Exit(-1)
		}
	}
}

func printVersion() {
	fmt.Printf(Cyan("p0d %s\n").String(), p0d.Version)
}

func printUsage() {
	p0d.PrintLogo()
	printVersion()
	fmt.Print("usage: p0d [-f flag] [URL]\n\n flags:\n")
	flag.PrintDefaults()
}
