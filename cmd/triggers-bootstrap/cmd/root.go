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

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tektoncd/triggers/pkg/bootstrap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	rootCmd = &cobra.Command{
		Use:   "triggers-bootstrap",
		Short: "Bootstrap Tekton Triggers for production use",
		Long: `Bootstrap Tekton Triggers for production use with full GitHub integration.

This command provides a complete production setup:

Examples:
  triggers-bootstrap
  
  # Quick setup with parameters
  triggers-bootstrap --github-repo owner/repo --domain myapp.com --github-token ghp_xxx`,
		RunE: rootRun,
	}

	// Command line flags
	namespace      string
	provider       string
	installDeps    bool
	createExamples bool
	setupIngress   bool
	dryRun         bool
	kubeconfig     string
	skipInstall    bool
	verbose        bool

	// Production configuration
	githubRepo    string
	githubToken   string
	publicDomain  string
	webhookSecret string
	interactive   bool
	forceSetup    bool
)

func init() {
	// Basic flags
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "getting-started", "Target namespace for Triggers resources")
	rootCmd.Flags().StringVar(&provider, "provider", "github", "Git provider (github|gitlab|bitbucket)")
	rootCmd.Flags().BoolVar(&installDeps, "install-deps", true, "Install Tekton Pipelines if not present")
	rootCmd.Flags().BoolVar(&createExamples, "create-examples", true, "Create example Pipeline, Task, and Trigger resources")
	rootCmd.Flags().BoolVar(&setupIngress, "setup-ingress", true, "Configure production ingress for EventListeners")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be created without applying")
	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	rootCmd.Flags().BoolVar(&skipInstall, "skip-install", false, "Skip Triggers installation, only create resources")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// configuration flags
	rootCmd.Flags().StringVar(&githubRepo, "github-repo", "", "GitHub repository (owner/repo)")
	rootCmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub personal access token")
	rootCmd.Flags().StringVar(&publicDomain, "domain", "", "Public domain for webhooks (e.g. myapp.com)")
	rootCmd.Flags().StringVar(&webhookSecret, "webhook-secret", "", "Webhook secret (auto-generated if empty)")
	rootCmd.Flags().BoolVar(&interactive, "interactive", true, "Interactive setup (disable for CI/CD)")
	rootCmd.Flags().BoolVar(&forceSetup, "force-setup", false, "Force setup even if webhooks exist")
}

func rootRun(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Check and install dependencies
	fmt.Println("Starting Tekton Triggers bootstrap...")

	// Build a temporary config just for installation checks
	tempConfig := &bootstrap.Config{
		Namespace:   namespace,
		DryRun:      dryRun,
		Verbose:     verbose,
		InstallDeps: installDeps,
		SkipInstall: skipInstall,
	}

	// Create kubernetes config for dependency checks
	var config *rest.Config
	var err error
	if !dryRun {
		if kubeconfig != "" {
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		} else {
			config, err = rest.InClusterConfig()
		}
		if err != nil {
			return fmt.Errorf("failed to build kubeconfig: %w", err)
		}
	}
	tempConfig.KubeConfig = config

	// Create installer for dependency checks
	var kubeClient kubernetes.Interface
	if !dryRun {
		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", err)
		}
	}

	installer := bootstrap.NewInstaller(kubeClient, tempConfig)

	// Check and install Tekton Pipelines
	fmt.Println("====Checking Tekton Pipelines...")
	if err := installer.InstallTektonPipelines(ctx); err != nil {
		return fmt.Errorf("failed to ensure pipelines: %w", err)
	}

	// Check and install Tekton Triggers
	if !skipInstall {
		fmt.Println("====Checking Tekton Triggers...")
		if err := installer.InstallTriggers(ctx); err != nil {
			return fmt.Errorf("failed to install triggers: %w", err)
		}
	}

	fmt.Println("Tekton dependencies are ready!\n")

	// Create getting-started resources (following docs/getting-started/README.md)
	fmt.Println("====Setting up Tekton Triggers resources...")

	// Create bootstrap configuration
	cfg := &bootstrap.Config{
		Namespace:      namespace,
		Provider:       provider,
		InstallDeps:    true,  // TRUE to create RBAC and Trigger resources!
		CreateExamples: true,  // Always create examples (docs/getting-started)
		SetupIngress:   false, // Don't setup ingress without GitHub
		DryRun:         dryRun,
		SkipInstall:    true, // Already done above
		Verbose:        verbose,
		KubeConfig:     config,

		// NO GitHub config, resources first!
		GitHubRepo:    "",
		GitHubToken:   "",
		PublicDomain:  "",
		WebhookSecret: "",
		ForceSetup:    false,
	}

	// Create bootstrapper
	bootstrapper, err := bootstrap.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create bootstrapper: %w", err)
	}

	// Run bootstrap (create namespace, RBAC, EventListener, Pipeline)
	if err := bootstrapper.Run(ctx); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	fmt.Println("=-=-=-= Tekton Triggers setup complete! =-=-=-=")

	fmt.Print("\nConfigure GitHub integration: ")

	if err := runInteractiveSetup(); err != nil {
		fmt.Printf("⚠️  GitHub setup failed: %v\n", err)
		return nil
	}

	fmt.Println("====Setting up GitHub integration...")

	// Update config for GitHub-only operations (skip core setup)
	cfg.InstallDeps = false    // Don't reinstall dependencies
	cfg.CreateExamples = false // Don't recreate examples
	cfg.GitHubRepo = githubRepo
	cfg.GitHubToken = githubToken
	cfg.PublicDomain = publicDomain
	cfg.WebhookSecret = webhookSecret
	cfg.SetupIngress = setupIngress

	// Create new bootstrapper for GitHub operations only
	githubBootstrapper, err := bootstrap.New(cfg)
	if err != nil {
		fmt.Printf("⚠️  GitHub setup failed: %v\n", err)
	} else {
		// This will skip core setup because InstallDeps=false and CreateExamples=false
		if err := githubBootstrapper.Run(ctx); err != nil {
			fmt.Printf("⚠️  GitHub integration failed: %v\n", err)
		} else {
			fmt.Println("GitHub integration complete!")
		}
	}

	fmt.Println("\nTekton Triggers bootstrap completed successfully!")
	return nil
}

// runInteractiveSetup runs the interactive configuration
func runInteractiveSetup() error {
	scanner := bufio.NewScanner(os.Stdin)

	// Get GitHub repository
	if githubRepo == "" {
		fmt.Print("\n Enter your GitHub repository (owner/repo): ")
		if scanner.Scan() {
			githubRepo = strings.TrimSpace(scanner.Text())
		}
		if githubRepo == "" {
			return fmt.Errorf("GitHub repository is required")
		}
	}

	// Get public domain
	if publicDomain == "" {
		fmt.Print(" Enter your public route URL (e.g. myapp.example.com): ")
		if scanner.Scan() {
			publicDomain = strings.TrimSpace(scanner.Text())
		}
		if publicDomain == "" {
			return fmt.Errorf("public domain is required for webhooks")
		}
	}

	// Get GitHub token
	if githubToken == "" {
		fmt.Print(" Enter your GitHub personal access token: ")
		if scanner.Scan() {
			githubToken = strings.TrimSpace(scanner.Text())
		}
		if githubToken == "" {
			return fmt.Errorf("GitHub token is required for webhook creation")
		}
	}

	return nil
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
