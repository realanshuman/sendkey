// Command sendkey is a zero-knowledge, burn-after-reading secret sharing
// tool. One static binary is both the server and the client: secrets are
// encrypted before they leave the sender, the server only ever stores
// ciphertext it cannot read, and the ciphertext is destroyed the moment it
// is retrieved.
package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "1.0.0"

const usage = `sendkey — zero-knowledge, burn-after-reading secret sharing

Usage:
  sendkey serve [flags]          run the server
  sendkey send  [flags] [secret] encrypt a secret and print a one-time link
  sendkey get   [flags] <link>   fetch and decrypt a secret from a link
  sendkey version                print the version

Run "sendkey <command> -h" for command flags.

The decryption key travels only in the URL fragment (#...), which browsers
and this CLI never send over the network — the server cannot read anything.
`

func main() {
	args := os.Args[1:]
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}

	switch sub {
	case "serve":
		cmdServe(args)
	case "send":
		cmdSend(args)
	case "get":
		cmdGet(args)
	case "version":
		fmt.Println("sendkey " + version)
	case "", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "sendkey: unknown command %q\n\n%s", sub, usage)
		os.Exit(2)
	}
}
