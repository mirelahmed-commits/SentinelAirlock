package policy

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version   int `yaml:"version"`
	Workspace struct {
		Ignore []string `yaml:"ignore"`
	} `yaml:"workspace"`
	Policy struct {
		DenyRead   []string `yaml:"deny_read"`
		DenyWrite  []string `yaml:"deny_write"`
		AllowWrite []string `yaml:"allow_write"`
	} `yaml:"policy"`
	Network struct {
		Mode      string   `yaml:"mode"`
		Allowlist []string `yaml:"allowlist"`
	} `yaml:"network"`
	Signing struct {
		PrivateKey string `yaml:"private_key"`
		PublicKey  string `yaml:"public_key"`
		KeyID      string `yaml:"key_id"`
	} `yaml:"signing"`
	Team struct {
		Name          string `yaml:"name"`
		DefaultWorker string `yaml:"default_worker"`
		IndexPath     string `yaml:"index_path"`
	} `yaml:"team"`
	Defaults struct {
		Agent      string `yaml:"agent"`
		Mode       string `yaml:"mode"`
		PolicyPack string `yaml:"policy_pack"`
		Sandbox    string `yaml:"sandbox"`
	} `yaml:"defaults"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
