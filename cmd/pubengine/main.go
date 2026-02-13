package main

import (
	"fmt"
	"os"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: pubengine new <project-name>")
			os.Exit(1)
		}
		if err := runNew(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("pubengine %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`pubengine - A blog publishing engine built with Go, Echo, and templ

Usage:
  pubengine <command> [arguments]

Commands:
  new <name>    Create a new pubengine project
  version       Print the pubengine version
  help          Show this help message

Examples:
  pubengine new myblog
  pubengine new github.com/user/myblog`)
}
