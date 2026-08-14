// Counts an MCP tool catalog's token cost with the same method the in-repo
// budget test uses: compact JSON of [{name, description, inputSchema}] counted
// by the Caveman offline o200k_base counter. Reads fetch-tools.mjs output on
// stdin; prints per-tool and total counts. Every figure is inferred.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/JuliusBrussee/caveman/engine/tokens"
)

type catalog struct {
	Server *struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"server"`
	Tools []map[string]any `json:"tools"`
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var c catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	counter := tokens.Default()
	if c.Server != nil {
		fmt.Printf("server: %s %s\n", c.Server.Name, c.Server.Version)
	}
	for _, tool := range c.Tools {
		single, err := json.Marshal([]map[string]any{tool})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("  %-40v %6d tokens\n", tool["name"], counter.Count(single))
	}
	full, err := json.Marshal(c.Tools)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("tools: %d  total: %d tokens (basis: inferred, o200k_base, compact JSON [{name,description,inputSchema}])\n",
		len(c.Tools), counter.Count(full))
}
