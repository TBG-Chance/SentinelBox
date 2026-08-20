//go:build !linux

package main

import "fmt"

func requireFirewallPrivileges() error {
	return fmt.Errorf("live nftables operations are supported only on Linux appliances")
}
