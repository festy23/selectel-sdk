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
	for _, service := range local.Services() {
		if service.Discovery != nil {
			fmt.Printf("service=%s availability=%s inventory=unknown evidence=%s followups=%d\n",
				service.ID, service.Availability, service.Discovery.EvidenceRef, len(service.Discovery.FollowupIssues))
		}
	}
}
