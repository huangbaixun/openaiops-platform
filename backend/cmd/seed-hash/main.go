package main

import (
	"fmt"
	"os"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: seed-hash <plaintext>")
		os.Exit(2)
	}
	h, err := apikey.Hash(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash error:", err)
		os.Exit(1)
	}
	fmt.Println(h)
}
