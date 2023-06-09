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

func TestMainFuncWithCfg(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	os.Args = append([]string{"-C"}, "-C", "../../examples/config_get_tls_http2_google.yml")
	main()
}

func TestPrintusage(t *testing.T) {
	printUsage()
}
