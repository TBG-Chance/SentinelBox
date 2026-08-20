package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)

func (c Config) Validate() error {
	var failures []error
	add := func(condition bool, format string, values ...any) {
		if condition {
			failures = append(failures, fmt.Errorf(format, values...))
		}
	}

	add(c.Version != CurrentVersion, "configuration version must be %d", CurrentVersion)

	host, portText, err := net.SplitHostPort(strings.TrimSpace(c.Server.Listen))
	if err != nil {
		failures = append(failures, fmt.Errorf("server.listen must be a host:port address: %w", err))
	} else {
		port, portErr := strconv.Atoi(portText)
		add(portErr != nil || port < 1 || port > 65535, "server.listen port must be between 1 and 65535")
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(host)
		add(host != "localhost" && (ip == nil || !ip.IsLoopback()), "server.listen must use a loopback address until management authentication is implemented")
	}

	validateDuration := func(name string, value time.Duration, maximum time.Duration) {
		add(value <= 0, "%s must be positive", name)
		add(value > maximum, "%s must not exceed %s", name, maximum)
	}
	validateDuration("server.read_timeout", c.Server.ReadTimeout.Duration, 5*time.Minute)
	validateDuration("server.write_timeout", c.Server.WriteTimeout.Duration, 5*time.Minute)
	validateDuration("server.idle_timeout", c.Server.IdleTimeout.Duration, 30*time.Minute)
	validateDuration("server.shutdown_timeout", c.Server.ShutdownTimeout.Duration, 5*time.Minute)
	validateDuration("server.health_timeout", c.Server.HealthTimeout.Duration, 30*time.Second)

	add(!isAbsolutePath(c.Database.Path), "database.path must be absolute")
	validateDuration("database.busy_timeout", c.Database.BusyTimeout.Duration, time.Minute)
	add(c.Database.MaxOpenConnections < 1 || c.Database.MaxOpenConnections > 16, "database.max_open_connections must be between 1 and 16")

	add(c.Network.Mode != "routed", "network.mode must be routed for the MVP")
	prefix, prefixErr := netip.ParsePrefix(c.Network.LANAddress)
	add(prefixErr != nil || !prefix.Addr().Is4(), "network.lan_address must be a valid IPv4 prefix")
	if prefixErr == nil && prefix.Addr().Is4() {
		networkAddress := prefix.Masked().Addr()
		broadcastAddress := ipv4Broadcast(prefix)
		add(prefix.Bits() > 30, "network.lan_address must provide usable IPv4 host addresses")
		add(prefix.Addr() == networkAddress || prefix.Addr() == broadcastAddress, "network.lan_address must use a host address, not the network or broadcast address")
	}
	add(!isAbsolutePath(c.Network.NftBinary), "network.nft_binary must be absolute")
	add(filepath.Base(c.Network.NftBinary) != "nft", "network.nft_binary must name the nft executable")
	add(c.Network.EnableIPv6Forwarding, "IPv6 forwarding is unavailable until equivalent IPv6 policy and enforcement are implemented")
	if c.Network.WANInterface != "" {
		add(!interfaceNamePattern.MatchString(c.Network.WANInterface), "network.wan_interface is invalid")
	}
	if c.Network.LANInterface != "" {
		add(!interfaceNamePattern.MatchString(c.Network.LANInterface), "network.lan_interface is invalid")
	}
	if c.Network.Enabled {
		add(!interfaceNamePattern.MatchString(c.Network.WANInterface), "network.wan_interface is invalid")
		add(!interfaceNamePattern.MatchString(c.Network.LANInterface), "network.lan_interface is invalid")
		add(c.Network.WANInterface == c.Network.LANInterface, "network WAN and LAN interfaces must be different")
		add(!c.Network.EnableIPv4Forwarding, "network.enable_ipv4_forwarding must be true in routed mode")
	}

	start, startErr := netip.ParseAddr(c.DHCP.RangeStart)
	end, endErr := netip.ParseAddr(c.DHCP.RangeEnd)
	add(prefixErr != nil || startErr != nil || endErr != nil, "DHCP range and LAN prefix must be valid IP addresses")
	if prefixErr == nil && startErr == nil && endErr == nil {
		add(!start.Is4() || !end.Is4(), "DHCP range must use IPv4 addresses")
		add(!prefix.Contains(start) || !prefix.Contains(end), "DHCP range must be inside network.lan_address")
		add(start.Compare(end) > 0, "dhcp.range_start must not be greater than dhcp.range_end")
		add(start.Compare(prefix.Addr()) <= 0 && end.Compare(prefix.Addr()) >= 0, "DHCP range must not include the SentinelBox LAN address")
		if prefix.Addr().Is4() && start.Is4() && end.Is4() {
			networkAddress := prefix.Masked().Addr()
			broadcastAddress := ipv4Broadcast(prefix)
			add(start == networkAddress || end == broadcastAddress, "DHCP range must not include the network or broadcast address")
		}
	}
	validateDuration("dhcp.lease_duration", c.DHCP.LeaseDuration.Duration, 30*24*time.Hour)
	if c.DHCP.Enabled {
		add(!c.Network.Enabled, "dhcp.enabled requires network.enabled")
	}

	add(!isAbsolutePath(c.Suricata.EVEPath), "suricata.eve_path must be absolute")
	add(c.Suricata.StartPosition != "start" && c.Suricata.StartPosition != "end", "suricata.start_position must be start or end")
	add(c.Suricata.MaxEventBytes < 1024 || c.Suricata.MaxEventBytes > 10*1024*1024, "suricata.max_event_bytes must be between 1024 and 10485760")
	validateDuration("suricata.poll_interval", c.Suricata.PollInterval.Duration, time.Minute)

	add(c.Risk.WatchThreshold < 0 || c.Risk.WatchThreshold > 100, "risk.watch_threshold must be between 0 and 100")
	add(c.Risk.RestrictThreshold < 0 || c.Risk.RestrictThreshold > 100, "risk.restrict_threshold must be between 0 and 100")
	add(c.Risk.ContainThreshold < 0 || c.Risk.ContainThreshold > 100, "risk.contain_threshold must be between 0 and 100")
	add(!(c.Risk.WatchThreshold < c.Risk.RestrictThreshold && c.Risk.RestrictThreshold < c.Risk.ContainThreshold), "risk thresholds must be strictly increasing")
	validateDuration("risk.correlation_window", c.Risk.CorrelationWindow.Duration, 24*time.Hour)
	validateDuration("risk.recalculation_interval", c.Risk.RecalculationInterval.Duration, time.Hour)

	add(c.Response.AutomaticRestriction || c.Response.AutomaticContainment, "automatic response must remain disabled until the verified network enforcer milestone")
	add(!c.Response.ContainmentRequiresManualRelease, "containment_requires_manual_release must remain true for the MVP")
	add(!c.Management.BindLANOnly, "management.bind_lan_only must remain true")
	add(c.Management.AllowWAN, "management.allow_wan must remain false")

	level := strings.ToLower(strings.TrimSpace(c.Logging.Level))
	add(level != "debug" && level != "info" && level != "warn" && level != "error", "logging.level must be debug, info, warn, or error")
	format := strings.ToLower(strings.TrimSpace(c.Logging.Format))
	add(format != "json" && format != "text", "logging.format must be json or text")

	if len(failures) > 0 {
		return fmt.Errorf("invalid configuration: %w", errors.Join(failures...))
	}
	return nil
}

func isAbsolutePath(value string) bool {
	value = strings.TrimSpace(value)
	return filepath.IsAbs(value) || strings.HasPrefix(value, "/")
}

func ipv4Broadcast(prefix netip.Prefix) netip.Addr {
	address := prefix.Masked().Addr().As4()
	hostBits := 32 - prefix.Bits()
	value := uint32(address[0])<<24 | uint32(address[1])<<16 | uint32(address[2])<<8 | uint32(address[3])
	if hostBits > 0 {
		value |= uint32(1)<<hostBits - 1
	}
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}
