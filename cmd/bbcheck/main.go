// Command bbcheck reads a message from stdin and prints Analyze JSON.
// Usage: echo "message" | go run ./cmd/bbcheck [-sender NAME]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"scam-checker/engine"
)

func main() {
	sender := flag.String("sender", "", "sender name/number")
	flag.Parse()
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bbcheck: read stdin: %v\n", err)
		os.Exit(1)
	}
	res := engine.Analyze(string(data), *sender)
	b, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bbcheck: marshal: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
