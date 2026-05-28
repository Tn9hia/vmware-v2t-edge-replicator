package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds VCD authentication credentials and endpoints
type Config struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	Org      string `json:"org"`
}

// LoadFromFile reads credentials from a passfile.
// Format of passfile (one key=value per line):
//
//	host=https://hht-lab-vcd-01.cloud.lab
//	username=nghialt
//	password=P@ssw0rd1234!#@
//	org=System
func LoadFromFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open passfile %q: %w", path, err)
	}
	defer f.Close()

	cfg := &Config{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "host":
			cfg.Host = val
		case "username":
			cfg.Username = val
		case "password":
			cfg.Password = val
		case "org":
			cfg.Org = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading passfile: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	missing := []string{}
	if c.Host == "" {
		missing = append(missing, "host")
	}
	if c.Username == "" {
		missing = append(missing, "username")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	if c.Org == "" {
		// Default to System if not specified
		c.Org = "System"
	}
	if len(missing) > 0 {
		return fmt.Errorf("passfile missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
