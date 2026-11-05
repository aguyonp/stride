// Command stride is a minimal stub in the domain branch: the platform workstream owns the real
// entrypoint (auth, scheduler, server) and wires domain.New / domainapi.Register.
package main

import (
	"fmt"
	"os"
	_ "time/tzdata"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "stride: domain branch stub, the platform entrypoint replaces this file")
	os.Exit(2)
}
