// Command openapi exports or checks the portable API specification without
// starting PostgreSQL, Firebase, workers or the application HTTP server.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/Hostel-Hive/hostelhive-backend/internal/apidocs"
)

func main() {
	out := flag.String("output", "", "Write JSON to this file (otherwise stdout)")
	check := flag.String("check", "", "Fail if this file differs from the current contract")
	flag.Parse()
	data := apidocs.JSON()
	if *check != "" {
		current, err := os.ReadFile(*check)
		if err != nil || !bytes.Equal(bytes.ReplaceAll(current, []byte("\r\n"), []byte("\n")), data) {
			fmt.Fprintln(os.Stderr, "OpenAPI snapshot is stale; run go run ./cmd/openapi -output docs/openapi.json")
			os.Exit(1)
		}
		return
	}
	if *out != "" {
		if err := os.WriteFile(*out, data, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
