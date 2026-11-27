// Command emb-commands renders the command contract as JSON on stdout.
// `just commands-gen` redirects it into commands.json and `just commands-check`
// diffs it, so the output must stay byte-stable.
package main

import (
	"fmt"
	"os"

	"github.com/elcuervo/emb/internal/contract"
)

func main() {
	out, err := contract.Render()
	if err != nil {
		fmt.Fprintln(os.Stderr, "emb-commands:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintln(os.Stderr, "emb-commands:", err)
		os.Exit(1)
	}
}
