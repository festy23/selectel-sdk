// Discover catalog availability without credentials or Selectel requests.
package main

import (
	"fmt"
	"os"

	"github.com/festy23/selectel-sdk/catalog"
)

func main() {
	local := catalog.Default()
	operations, err := local.List(catalog.Filter{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog discovery failed")
		os.Exit(1)
	}
	fmt.Printf("catalog=ok operations=%d cloud_requests=0\n", len(operations))
	audit := local.Audit()
	for _, source := range audit.Sources {
		fmt.Printf("source=%s revision=%s pinned_operations=%d implemented=%d partial=%d missing_bindings=%d\n", source.URI, source.Revision, source.Documented, source.Implemented, source.Partial, len(source.UnimplementedIDs))
	}
}
