package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// AccountConfig holds credentials for a single Parro account.
type AccountConfig struct {
	RefreshToken string `json:"refresh_token"`
	GuardianID   string `json:"guardian_id"`
	Username     string `json:"username,omitempty"`
}

// Config is the top-level config file structure.
type Config struct {
	Accounts map[string]AccountConfig `json:"accounts"`
}

// Path returns the config file path ($XDG_CONFIG_HOME/parro/config.json).
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "parro", "config.json")
}

// Load reads the config file. Returns an empty Config if the file doesn't exist.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Accounts: map[string]AccountConfig{}}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = map[string]AccountConfig{}
	}
	return &cfg, nil
}

// Save writes the config file atomically (tmp + rename).
func (c *Config) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// Set adds or updates an account in the config.
func (c *Config) Set(acct AccountConfig) {
	c.Accounts[acct.GuardianID] = acct
}

// Get returns the account config for the given guardian ID.
func (c *Config) Get(id string) (AccountConfig, bool) {
	acct, ok := c.Accounts[id]
	return acct, ok
}

// Find returns the account matching the query, which can be a guardian ID or username.
func (c *Config) Find(query string) (AccountConfig, bool) {
	if acct, ok := c.Accounts[query]; ok {
		return acct, true
	}
	for _, acct := range c.Accounts {
		if acct.Username == query {
			return acct, true
		}
	}
	return AccountConfig{}, false
}

// List returns all accounts sorted by guardian ID.
func (c *Config) List() []AccountConfig {
	ids := make([]string, 0, len(c.Accounts))
	for id := range c.Accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]AccountConfig, len(ids))
	for i, id := range ids {
		result[i] = c.Accounts[id]
	}
	return result
}

// Only returns the single account if exactly one exists. Returns an error otherwise.
func (c *Config) Only() (AccountConfig, error) {
	switch len(c.Accounts) {
	case 0:
		return AccountConfig{}, fmt.Errorf("no accounts configured — run 'parro login' first")
	case 1:
		for _, acct := range c.Accounts {
			return acct, nil
		}
	}
	ids := make([]string, 0, len(c.Accounts))
	for id := range c.Accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return AccountConfig{}, fmt.Errorf("multiple accounts configured — use -a to select: %v", ids)
}
