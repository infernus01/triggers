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
	"fmt"

	"k8s.io/client-go/rest"
)

// Config holds configuration for the bootstrap process
type Config struct {
	// Namespace is the target namespace for Triggers resources
	Namespace string

	// Provider is the Git provider (github, gitlab, bitbucket)
	Provider string

	// InstallDeps determines whether to install Tekton Pipelines if not present
	InstallDeps bool

	// CreateExamples determines whether to create example resources
	CreateExamples bool

	// SetupIngress determines whether to configure ingress
	SetupIngress bool

	// DryRun shows what would be created without applying
	DryRun bool

	// SkipInstall skips Triggers installation, only creates resources
	SkipInstall bool

	// Verbose enables detailed output
	Verbose bool

	// KubeConfig is the Kubernetes client configuration
	KubeConfig *rest.Config

	// configuration
	GitHubRepo    string // GitHub repository (owner/repo)
	GitHubToken   string // GitHub personal access token
	PublicDomain  string // Public domain for webhooks
	WebhookSecret string // Webhook secret
	ForceSetup    bool   // Force setup even if webhooks exist
}

// Validate validates the bootstrap configuration
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	supportedProviders := map[string]bool{
		"github":    true,
		"gitlab":    true,
		"bitbucket": true,
	}

	if !supportedProviders[c.Provider] {
		return fmt.Errorf("unsupported provider: %s (supported: github, gitlab, bitbucket)", c.Provider)
	}

	// Only require kubeconfig if not in dry-run mode
	if !c.DryRun && c.KubeConfig == nil {
		return fmt.Errorf("kubeconfig is required (use --kubeconfig or try --dry-run to preview)")
	}

	// validation
	if c.SetupIngress {
		if c.GitHubRepo == "" {
			return fmt.Errorf("GitHub repository is required for setup (use --github-repo)")
		}
		if c.PublicDomain == "" {
			return fmt.Errorf("public domain is required for webhooks (use --domain)")
		}
		if c.GitHubToken == "" {
			return fmt.Errorf("GitHub token is required for webhook creation (use --github-token)")
		}
	}

	return nil
}

// TemplateData holds data for rendering templates
type TemplateData struct {
	Namespace      string
	Name           string
	ServiceAccount string
	Provider       string
}
