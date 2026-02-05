package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"otter-ai/internal/governance"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: keytool <command> [args]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  generate <data-dir>    Generate new key pair")
		fmt.Println("  show <data-dir>        Show public key")
		fmt.Println("  export <data-dir>      Export public key as hex")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "generate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: keytool generate <data-dir>")
			os.Exit(1)
		}
		dataDir := os.Args[2]
		generateKeys(dataDir)

	case "show":
		if len(os.Args) < 3 {
			fmt.Println("Usage: keytool show <data-dir>")
			os.Exit(1)
		}
		dataDir := os.Args[2]
		showPublicKey(dataDir)

	case "export":
		if len(os.Args) < 3 {
			fmt.Println("Usage: keytool export <data-dir>")
			os.Exit(1)
		}
		dataDir := os.Args[2]
		exportPublicKey(dataDir)

	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func generateKeys(dataDir string) {
	cs, err := governance.RegenerateKeys(dataDir)
	if err != nil {
		fmt.Printf("Error generating keys: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ New key pair generated")
	fmt.Printf("Public Key: %s\n", governance.ExportPublicKey(cs))
	fmt.Printf("Stored in: %s/otter.key\n", dataDir)
}

func showPublicKey(dataDir string) {
	cs, err := governance.LoadOrGenerateKeys(dataDir)
	if err != nil {
		fmt.Printf("Error loading keys: %v\n", err)
		os.Exit(1)
	}

	pubKeyBytes := cs.GetPublicKey()
	fmt.Println("Public Key (hex):")
	fmt.Println(hex.EncodeToString(pubKeyBytes))
	fmt.Println("")
	fmt.Printf("Length: %d bytes\n", len(pubKeyBytes))
}

func exportPublicKey(dataDir string) {
	cs, err := governance.LoadOrGenerateKeys(dataDir)
	if err != nil {
		fmt.Printf("Error loading keys: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(governance.ExportPublicKey(cs))
}
