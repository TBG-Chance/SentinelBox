package config

import "time"

const CurrentVersion = 1

type Duration struct {
	time.Duration
}

func NewDuration(value time.Duration) Duration {
	return Duration{Duration: value}
}

func (d *Duration) UnmarshalText(value []byte) error {
	parsed, err := time.ParseDuration(string(value))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

type Config struct {
	Version    int              `toml:"version"`
	Server     ServerConfig     `toml:"server"`
	Database   DatabaseConfig   `toml:"database"`
	Network    NetworkConfig    `toml:"network"`
	DHCP       DHCPConfig       `toml:"dhcp"`
	Suricata   SuricataConfig   `toml:"suricata"`
	Risk       RiskConfig       `toml:"risk"`
	Response   ResponseConfig   `toml:"response"`
	Management ManagementConfig `toml:"management"`
	Logging    LoggingConfig    `toml:"logging"`
}

type ServerConfig struct {
	Listen          string   `toml:"listen"`
	ReadTimeout     Duration `toml:"read_timeout"`
	WriteTimeout    Duration `toml:"write_timeout"`
	IdleTimeout     Duration `toml:"idle_timeout"`
	ShutdownTimeout Duration `toml:"shutdown_timeout"`
	HealthTimeout   Duration `toml:"health_timeout"`
}

type DatabaseConfig struct {
	Path               string   `toml:"path"`
	BusyTimeout        Duration `toml:"busy_timeout"`
	MaxOpenConnections int      `toml:"max_open_connections"`
}

type NetworkConfig struct {
	Enabled              bool   `toml:"enabled"`
	Mode                 string `toml:"mode"`
	WANInterface         string `toml:"wan_interface"`
	LANInterface         string `toml:"lan_interface"`
	LANAddress           string `toml:"lan_address"`
	NftBinary            string `toml:"nft_binary"`
	EnableIPv4Forwarding bool   `toml:"enable_ipv4_forwarding"`
	EnableIPv6Forwarding bool   `toml:"enable_ipv6_forwarding"`
}

type DHCPConfig struct {
	Enabled       bool     `toml:"enabled"`
	RangeStart    string   `toml:"range_start"`
	RangeEnd      string   `toml:"range_end"`
	LeaseDuration Duration `toml:"lease_duration"`
}

type SuricataConfig struct {
	Enabled       bool     `toml:"enabled"`
	EVEPath       string   `toml:"eve_path"`
	StartPosition string   `toml:"start_position"`
	MaxEventBytes int64    `toml:"max_event_bytes"`
	PollInterval  Duration `toml:"poll_interval"`
}

type RiskConfig struct {
	WatchThreshold        int      `toml:"watch_threshold"`
	RestrictThreshold     int      `toml:"restrict_threshold"`
	ContainThreshold      int      `toml:"contain_threshold"`
	CorrelationWindow     Duration `toml:"correlation_window"`
	RecalculationInterval Duration `toml:"recalculation_interval"`
}

type ResponseConfig struct {
	AutomaticRestriction             bool `toml:"automatic_restriction"`
	AutomaticContainment             bool `toml:"automatic_containment"`
	ContainmentRequiresManualRelease bool `toml:"containment_requires_manual_release"`
}

type ManagementConfig struct {
	BindLANOnly bool `toml:"bind_lan_only"`
	AllowWAN    bool `toml:"allow_wan"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

func Default() Config {
	return Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			Listen:          "127.0.0.1:8080",
			ReadTimeout:     NewDuration(10 * time.Second),
			WriteTimeout:    NewDuration(15 * time.Second),
			IdleTimeout:     NewDuration(60 * time.Second),
			ShutdownTimeout: NewDuration(15 * time.Second),
			HealthTimeout:   NewDuration(2 * time.Second),
		},
		Database: DatabaseConfig{
			Path:               "/var/lib/sentinelbox/sentinel.db",
			BusyTimeout:        NewDuration(5 * time.Second),
			MaxOpenConnections: 4,
		},
		Network: NetworkConfig{
			Enabled:              false,
			Mode:                 "routed",
			LANAddress:           "192.168.50.1/24",
			NftBinary:            "/usr/sbin/nft",
			EnableIPv4Forwarding: true,
			EnableIPv6Forwarding: false,
		},
		DHCP: DHCPConfig{
			Enabled:       false,
			RangeStart:    "192.168.50.100",
			RangeEnd:      "192.168.50.199",
			LeaseDuration: NewDuration(12 * time.Hour),
		},
		Suricata: SuricataConfig{
			Enabled:       false,
			EVEPath:       "/var/log/suricata/eve.json",
			StartPosition: "end",
			MaxEventBytes: 256 * 1024,
			PollInterval:  NewDuration(500 * time.Millisecond),
		},
		Risk: RiskConfig{
			WatchThreshold:        30,
			RestrictThreshold:     60,
			ContainThreshold:      80,
			CorrelationWindow:     NewDuration(5 * time.Minute),
			RecalculationInterval: NewDuration(30 * time.Second),
		},
		Response: ResponseConfig{
			ContainmentRequiresManualRelease: true,
		},
		Management: ManagementConfig{
			BindLANOnly: true,
			AllowWAN:    false,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
