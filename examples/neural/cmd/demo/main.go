package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	// Run the reverse engineering example
	cmd := exec.Command("go", "run", "../reverse/main.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running reverse example: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n============================================================")
	fmt.Println("This demonstrates the power of the learn package:")
	fmt.Println("  1. You encode DOMAIN KNOWLEDGE (process structure)")
	fmt.Println("  2. You provide OBSERVATIONS (real data)")
	fmt.Println("  3. System DISCOVERS PARAMETERS (rates)")
	fmt.Println("============================================================")
}
