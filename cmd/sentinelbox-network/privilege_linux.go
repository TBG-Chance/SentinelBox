//go:build linux

package main

import (
	"fmt"
	"os"
)

func requireFirewallPrivileges() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("nftables inspection and changes require root on the appliance")
	}
	return nil
}
