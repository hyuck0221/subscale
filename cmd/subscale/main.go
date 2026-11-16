package main

import (
	"fmt"
	"github.com/hyuck0221/subscale/internal/cli"
	"os"
)

var version = "dev"

func main() {
	if e := cli.Run(os.Args[1:], version); e != nil {
		fmt.Fprintln(os.Stderr, "subscale:", e)
		os.Exit(1)
	}
}
