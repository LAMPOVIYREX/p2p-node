package node

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

type Config struct {
	NodeStateDir     string   `yaml:"node_state_dir"`
	ListenHost       string   `yaml:"listen_host"`
	ListenPort       int      `yaml:"listen_port"`
	BootstrapPeers   []string `yaml:"bootstrap_peers"`
	NodeIDBits       int      `yaml:"node_id_bits"`
	KBucketSize      int      `yaml:"k_bucket_size"`
	Alpha            int      `yaml:"alpha"`
	ConnectTimeoutMs int      `yaml:"connect_timeout_ms"`
	ReadTimeoutMs    int      `yaml:"read_timeout_ms"`
	PingTimeoutMs    int      `yaml:"ping_timeout_ms"`
	MaxFramePayload  int      `yaml:"max_frame_payload"`
	ProtocolVersion  int      `yaml:"protocol_version"`
	LogLevel         string   `yaml:"log_level"`
	AdvertiseHost    string   `yaml:"advertise_host"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	if c.MaxFramePayload == 0 {
		c.MaxFramePayload = transport.MaxFramePayload
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = transport.ProtocolVersion
	}
	if c.MaxFramePayload < 256 || c.MaxFramePayload > 16*1024*1024 {
		return nil, fmt.Errorf("max_frame_payload out of range: %d (must be 256..16777216)", c.MaxFramePayload)
	}
	if c.ProtocolVersion < 1 || c.ProtocolVersion > 255 {
		return nil, fmt.Errorf("protocol_version out of range: %d (must be 1..255)", c.ProtocolVersion)
	}

	if c.NodeIDBits != 0 && c.NodeIDBits != 256 {
		return nil, fmt.Errorf("node_id_bits must be 256")
	}
	if c.Alpha == 0 {
		c.Alpha = 3
	}
	if c.KBucketSize == 0 {
		c.KBucketSize = 4
	}
	if c.AdvertiseHost == "" {
		c.AdvertiseHost = c.ListenHost
	}
	return &c, nil
}

func (c *Config) ConnectTimeout() time.Duration {
	return time.Duration(c.ConnectTimeoutMs) * time.Millisecond
}
func (c *Config) ReadTimeout() time.Duration {
	return time.Duration(c.ReadTimeoutMs) * time.Millisecond
}
func (c *Config) PingTimeout() time.Duration {
	return time.Duration(c.PingTimeoutMs) * time.Millisecond
}
