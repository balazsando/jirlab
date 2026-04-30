package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration loaded from environment variables
// or an optional .env file.
type Config struct {
	JiraURL         string
	JiraAPIURL      string
	JiraEmail       string
	JiraToken       string
	JiraBoardID     string
	JiraProject     string // optional: project key e.g. METAAPI
	JiraMyAccountID string // optional: own Jira account ID for colour highlighting

	GitLabToken  string
	GitLabAPIURL string

	// AzureClientID is the Azure App Registration client ID for Microsoft Graph
	// (device code / delegated auth). Optional — Chats section is disabled if absent.
	AzureClientID string
}

// Load reads configuration from environment variables and an optional .env file.
// Environment variables always take precedence over the .env file values.
func Load() (*Config, error) {
	v := viper.New()

	// Map env var names to viper keys
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Attempt to load .env file (not required)
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	_ = v.ReadInConfig() // intentionally ignore missing file error

	cfg := &Config{
		JiraURL:      v.GetString("JIRA_URL"),
		JiraAPIURL:   v.GetString("JIRA_URL") + "/rest/api",
		JiraEmail:    v.GetString("JIRA_EMAIL"),
		JiraToken:    v.GetString("JIRA_TOKEN"),
		JiraBoardID:  v.GetString("JIRA_BOARD_ID"),
		GitLabToken:  v.GetString("GITLAB_TOKEN"),
		GitLabAPIURL: v.GetString("GITLAB_API_URL"),
		AzureClientID: v.GetString("AZURE_CLIENT_ID"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate ensures required fields are present.
func (c *Config) validate() error {
	required := map[string]string{
		"JIRA_URL":       c.JiraURL,
		"JIRA_API_URL":   c.JiraAPIURL,
		"JIRA_EMAIL":     c.JiraEmail,
		"JIRA_TOKEN":     c.JiraToken,
		"JIRA_BOARD_ID":  c.JiraBoardID,
		"GITLAB_TOKEN":   c.GitLabToken,
		"GITLAB_API_URL": c.GitLabAPIURL,
	}
	var missing []string
	for k, v := range required {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}
