// Command validate checks engine/rules.json schema, i18n, weights and
// regex patterns (RE2∩JS common subset). Exits non-zero on errors.
package main

import (
	"fmt"
	"os"

	"scam-checker/engine"
)

func main() {
	if err := engine.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "rules.json INVALID: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("rules.json OK")
}
