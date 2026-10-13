// Discover catalog availability without credentials or Selectel requests.
package main

import (
	"fmt"
	"os"

	"github.com/festy23/selectel-sdk/catalog"
)

func main() {
	operations, err := catalog.Default().List(catalog.Filter{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog discovery failed")
		os.Exit(1)
	}
	fmt.Printf("catalog=ok operations=%d cloud_requests=0\n", len(operations))
}
