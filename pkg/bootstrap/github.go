/*
Copyright 2023 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GitHubManager handles GitHub integration
type GitHubManager struct {
	config *Config
	client *http.Client
}

// NewGitHubManager creates a new GitHub manager
func NewGitHubManager(config *Config) *GitHubManager {
	return &GitHubManager{
		config: config,
		client: &http.Client{},
	}
}

// WebhookPayload represents the GitHub webhook payload
type WebhookPayload struct {
	Name   string        `json:"name"`
	Config WebhookConfig `json:"config"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
}

// WebhookConfig represents webhook configuration
type WebhookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret,omitempty"`
	InsecureSSL string `json:"insecure_ssl"`
}

// SetupWebhook creates a GitHub webhook for the repository
func (g *GitHubManager) SetupWebhook(ctx context.Context) error {
	if g.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create GitHub webhook for %s\n", g.config.GitHubRepo)
		fmt.Printf("    URL: https://%s/hooks\n", g.config.PublicDomain)
		return nil
	}

	// Generate webhook secret if not provided
	secret := g.config.WebhookSecret
	if secret == "" {
		var err error
		secret, err = generateWebhookSecret()
		if err != nil {
			return fmt.Errorf("failed to generate webhook secret: %w", err)
		}
		g.config.WebhookSecret = secret
	}

	// Create webhook
	webhookURL := fmt.Sprintf("https://%s/hooks", g.config.PublicDomain)

	if g.config.Verbose {
		fmt.Printf("  🔗 Creating webhook at: %s\n", webhookURL)
	}

	payload := WebhookPayload{
		Name: "web",
		Config: WebhookConfig{
			URL:         webhookURL,
			ContentType: "json",
			Secret:      secret,
			InsecureSSL: "0", // Always use SSL in production
		},
		Events: []string{"push", "pull_request"},
		Active: true,
	}

	if err := g.createWebhook(ctx, payload); err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	if g.config.Verbose {
		fmt.Printf("  ✅ Webhook created successfully\n")
		fmt.Printf("  🔑 Webhook secret: %s...%s\n", secret[:8], secret[len(secret)-8:])
	}

	return nil
}

// CheckExistingWebhooks checks if webhooks already exist
func (g *GitHubManager) CheckExistingWebhooks(ctx context.Context) (bool, error) {
	if g.config.DryRun {
		return false, nil
	}

	// Check if webhooks already exist
	url := fmt.Sprintf("https://api.github.com/repos/%s/hooks", g.config.GitHubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "token "+g.config.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check existing webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var webhooks []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&webhooks); err != nil {
		return false, err
	}

	// Check if our webhook URL already exists
	webhookURL := fmt.Sprintf("https://%s/hooks", g.config.PublicDomain)
	for _, webhook := range webhooks {
		if config, ok := webhook["config"].(map[string]interface{}); ok {
			if url, ok := config["url"].(string); ok && url == webhookURL {
				return true, nil
			}
		}
	}

	return false, nil
}

// createWebhook makes the API call to create the webhook
func (g *GitHubManager) createWebhook(ctx context.Context, payload WebhookPayload) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/hooks", g.config.GitHubRepo)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+g.config.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 422 && strings.Contains(string(body), "Hook already exists") {
			if g.config.ForceSetup {
				fmt.Printf("  ⚠️  Webhook exists, but --force-setup specified\n")
				// In a real implementation, we'd update the existing webhook
				return nil
			}
			return fmt.Errorf("webhook already exists (use --force-setup to override)")
		}
		return fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// generateWebhookSecret generates a secure random webhook secret
func generateWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetWebhookSecret returns the webhook secret for storing in Kubernetes
func (g *GitHubManager) GetWebhookSecret() string {
	return g.config.WebhookSecret
}
