package main

import (
	"flag"
	"os"
	"testing"
)

func TestMainFunc(t *testing.T) {
	os.Args = append([]string{"-v"}, "-v")
	main()
}

func TestMainFuncWithHelp(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = append([]string{"-h"}, "-h")
	main()
}

func TestMainFuncWithSkipInet(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = append([]string{"-s"}, "-s", "https://www.google.com")
	main()
}

func TestMainFuncWithDuration(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = append([]string{"-d"}, "-d", "3", "https://www.google.com")
	main()
}

func TestMainFuncWithCfg(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	os.Args = append([]string{"-C"}, "-C", "../../examples/config_get_tls_http2_google.yml")
	main()
}

func TestPrintusage(t *testing.T) {
	printUsage()
}
