package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Servers map[string]Server `yaml:"servers"`
}
type Server struct {
	Host      string    `yaml:"host"`
	Port      int       `yaml:"port"`
	Username  string    `yaml:"username"`
	Auth      Auth      `yaml:"auth"`
	Forwards  []Forward `yaml:"forwards"`
	Reconnect Reconnect `yaml:"reconnect"`
	Keepalive Keepalive `yaml:"keepalive"`
	Log       Log       `yaml:"log"`
}
type Auth struct {
	Type     string `yaml:"type"`
	Password string `yaml:"password"`
	TOTPSeed string `yaml:"totp_seed"`
}
type Forward struct {
	Name       string `yaml:"name"`
	LocalHost  string `yaml:"local_host"`
	LocalPort  int    `yaml:"local_port"`
	RemoteHost string `yaml:"remote_host"`
	RemotePort int    `yaml:"remote_port"`
}
type Reconnect struct {
	Enabled             bool `yaml:"enabled"`
	InitialDelaySeconds int  `yaml:"initial_delay_seconds"`
	MaxDelaySeconds     int  `yaml:"max_delay_seconds"`
	MaxRetries          int  `yaml:"max_retries"`
}
type Keepalive struct {
	Enabled         bool `yaml:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds"`
}
type Log struct {
	Level string `yaml:"level"`
}

func DefaultPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".ssh-tunnel", "config.yaml")
	}
	return "config.yaml"
}
func Load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, err
	}
	warnings := checkPerms(path)
	if len(cfg.Servers) == 0 {
		return nil, warnings, errors.New("configuration must contain at least one server")
	}
	for name, srv := range cfg.Servers {
		applyDefaults(&srv)
		if err := srv.Validate(); err != nil {
			return nil, warnings, fmt.Errorf("server %s: %w", name, err)
		}
		cfg.Servers[name] = srv
	}
	return &cfg, warnings, nil
}
func (c *Config) Server(name string) (Server, error) {
	s, ok := c.Servers[name]
	if !ok {
		return Server{}, fmt.Errorf("server %q not found", name)
	}
	return s, nil
}
func applyDefaults(s *Server) {
	if s.Port == 0 {
		s.Port = 22
	}
	if s.Reconnect.InitialDelaySeconds == 0 {
		s.Reconnect.InitialDelaySeconds = 3
	}
	if s.Reconnect.MaxDelaySeconds == 0 {
		s.Reconnect.MaxDelaySeconds = 60
	}
	if s.Keepalive.IntervalSeconds == 0 {
		s.Keepalive.IntervalSeconds = 30
	}
	for i := range s.Forwards {
		if s.Forwards[i].LocalHost == "" {
			s.Forwards[i].LocalHost = "127.0.0.1"
		}
		if s.Forwards[i].RemoteHost == "" {
			s.Forwards[i].RemoteHost = "127.0.0.1"
		}
		if s.Forwards[i].Name == "" {
			s.Forwards[i].Name = fmt.Sprintf("forward-%d", i+1)
		}
	}
}
func (s Server) Validate() error {
	if s.Host == "" {
		return errors.New("host is required")
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("port must be 1-65535")
	}
	if s.Username == "" {
		return errors.New("username is required")
	}
	if s.Auth.Type != "password_totp" {
		return errors.New("only auth.type password_totp is supported")
	}
	if s.Auth.Password == "" {
		return errors.New("auth.password is required")
	}
	if s.Auth.TOTPSeed == "" {
		return errors.New("auth.totp_seed is required")
	}
	if len(s.Forwards) == 0 {
		return errors.New("at least one forward is required")
	}
	for _, f := range s.Forwards {
		if net.ParseIP(f.LocalHost) == nil && f.LocalHost != "localhost" { /* hostnames allowed */
		}
		if f.LocalPort < 1 || f.LocalPort > 65535 || f.RemotePort < 1 || f.RemotePort > 65535 {
			return fmt.Errorf("forward %s ports must be 1-65535", f.Name)
		}
		if f.RemoteHost == "" {
			return fmt.Errorf("forward %s remote_host is required", f.Name)
		}
	}
	return nil
}
func checkPerms(path string) []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return []string{fmt.Sprintf("configuration file %s permissions are broader than 0600", path)}
	}
	return nil
}
