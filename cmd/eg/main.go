// Command eg is the command-line client for ErvisioRepo, compatible with
// GitHub's gh.
package main

import (
	"os"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/root"
)

func main() {
	os.Exit(root.Main(os.Args[1:]))
}
