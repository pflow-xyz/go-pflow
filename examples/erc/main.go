// Package main demonstrates defining ERC token standards as Petri net schemas
// and generating Solidity contracts from them.
//
// The tokenmodel package provides a declarative DSL for defining state machines
// where states are places (data containers) and actions are transitions
// (operations that transform state). Arcs connect states to actions, defining
// token flows.
//
// Usage:
//
//	go run ./examples/erc
//
// This will generate Solidity contracts for ERC-20, ERC-721, and ERC-1155.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pflow-xyz/go-pflow/codegen/solidity"
	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

func main() {
	type schemaEntry struct {
		name   string
		schema *tokenmodel.Schema
	}

	schemas := []schemaEntry{
		{"ERC20Token", NewERC020("ERC20Token")},
		{"ERC721Token", NewERC721("ERC721Token")},
		{"ERC1155Token", NewERC1155("ERC1155Token")},
	}

	outDir := "generated"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range schemas {
		// Generate Solidity
		sol := solidity.Generate(entry.schema)

		// Write to file
		filename := filepath.Join(outDir, entry.name+".sol")
		if err := os.WriteFile(filename, []byte(sol), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", filename, err)
			continue
		}

		// Print schema info
		fmt.Printf("Generated %s:\n", filename)
		fmt.Printf("  Version: %s\n", entry.schema.Version)
		fmt.Printf("  Content ID: %s\n", entry.schema.CID())
		fmt.Printf("  States: %d, Actions: %d, Arcs: %d\n",
			len(entry.schema.States), len(entry.schema.Actions), len(entry.schema.Arcs))
		fmt.Printf("  Size: %d bytes\n\n", len(sol))
	}

	fmt.Printf("Solidity contracts written to %s/\n", outDir)
}
