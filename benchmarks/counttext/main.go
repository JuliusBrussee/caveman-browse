// Counts stdin with the Caveman offline o200k_base counter. Used by the
// snapshot head-to-head so every representation is measured by one tokenizer.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/JuliusBrussee/caveman/engine/tokens"
)

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d tokens (%d bytes, basis: inferred, o200k_base)\n", tokens.Default().Count(raw), len(raw))
}
