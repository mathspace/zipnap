package gharunner

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the configuration for a GitHub Actions Runner activator.
type Config struct {
	Repos        []string `yaml:"repos"`
	RunnerLabels []string `yaml:"runner_labels"`
	TokenEnvVar  string   `yaml:"token_env_var,omitempty"`
}

func (c *Config) UnmarshalYAML(n *yaml.Node) error {
	type alias Config
	var cfg alias
	if err := n.Decode(&cfg); err != nil {
		return err
	}
	if cfg.TokenEnvVar == "" {
		cfg.TokenEnvVar = "GITHUB_TOKEN"
	}
	*c = Config(cfg)
	return nil
}

func splitOwnerRepo(full string) (owner, repo string, err error) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo format: expected 'owner/repo'")
	}
	return parts[0], parts[1], nil
}

// Validate checks the GitHub Actions Runner configuration for validity.
func (c *Config) Validate() error {
	if len(c.Repos) == 0 {
		return fmt.Errorf("repos must not be empty")
	}
	if len(c.RunnerLabels) == 0 {
		return fmt.Errorf("runner_labels must not be empty")
	}
	for _, repo := range c.Repos {
		if _, _, err := splitOwnerRepo(repo); err != nil {
			return fmt.Errorf("invalid repo '%s': %w", repo, err)
		}
	}
	return nil
}
