package main

import (
	"os"
	"testing"
)

func TestMainFunc(t *testing.T) {
	os.Args = append([]string{"-v"}, "-v")
	main()
}

func TestPrintusage(t *testing.T) {
	printUsage()
}
