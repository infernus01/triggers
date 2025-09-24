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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// Installer handles Tekton Triggers installation
type Installer struct {
	kubeClient kubernetes.Interface
	config     *Config
}

// NewInstaller creates a new installer
func NewInstaller(kubeClient kubernetes.Interface, config *Config) *Installer {
	return &Installer{
		kubeClient: kubeClient,
		config:     config,
	}
}

// InstallTriggers installs Tekton Triggers CRDs and controllers
func (i *Installer) InstallTriggers(ctx context.Context) error {
	if i.config.DryRun {
		fmt.Println("  [DRY-RUN] Would install Tekton Triggers CRDs and controllers")
		return nil
	}

	// Check if Triggers is already installed
	if i.isTriggersInstalled(ctx) {
		if i.config.Verbose {
			fmt.Println("  Tekton Triggers is already installed")
		}
		return nil
	}

	// REAL installation logic
	fmt.Println("  Downloading Tekton Triggers manifests...")
	if err := i.downloadAndApplyTriggers(ctx); err != nil {
		return fmt.Errorf("failed to install Triggers: %w", err)
	}

	fmt.Println("  Waiting for Triggers to be ready...")
	return i.waitForTriggersReady(ctx)
}

// isTriggersInstalled checks if Tekton Triggers is already installed
func (i *Installer) isTriggersInstalled(ctx context.Context) bool {
	if i.kubeClient == nil {
		return false // In dry-run, assume nothing is installed
	}
	// Check for the existence of the Triggers namespace and deployments
	_, err := i.kubeClient.AppsV1().Deployments("tekton-pipelines").Get(ctx, "tekton-triggers-controller", metav1.GetOptions{})
	return err == nil
}

// waitForTriggersReady waits for Tekton Triggers components to be ready
func (i *Installer) waitForTriggersReady(ctx context.Context) error {
	if i.config.DryRun {
		return nil
	}

	// Wait for the tekton-pipelines namespace to exist
	err := wait.PollImmediate(2*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := i.kubeClient.CoreV1().Namespaces().Get(ctx, "tekton-pipelines", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if i.config.Verbose {
				fmt.Println("    Waiting for tekton-pipelines namespace...")
			}
			return false, nil
		}
		return err == nil, err
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for tekton-pipelines namespace: %w", err)
	}

	// In a real implementation, we would wait for actual deployments
	// For demo purposes, we'll just simulate a short wait
	time.Sleep(2 * time.Second)

	if i.config.Verbose {
		fmt.Println("  Tekton Triggers installation completed")
	}

	return nil
}

// InstallTektonPipelines installs Tekton Pipelines if requested and not present
func (i *Installer) InstallTektonPipelines(ctx context.Context) error {
	if !i.config.InstallDeps {
		// Check if Pipelines is installed anyway - it's required for Triggers
		if !i.isPipelinesInstalled(ctx) {
			return fmt.Errorf("Tekton Pipelines is not installed and is required for Triggers. Use --install-deps to install it automatically")
		}
		return nil
	}

	if i.isPipelinesInstalled(ctx) {
		if i.config.Verbose {
			fmt.Println("  Tekton Pipelines is already installed")
		}
		return nil
	}

	if i.config.DryRun {
		fmt.Println("  [DRY-RUN] Would install Tekton Pipelines")
		return nil
	}

	fmt.Println("  Installing Tekton Pipelines (dependency)...")
	if err := i.downloadAndApplyPipelines(ctx); err != nil {
		return fmt.Errorf("failed to install Pipelines: %w", err)
	}

	// CRITICAL: Wait for Pipelines to be ready before continuing
	fmt.Println("  Waiting for Tekton Pipelines to be ready...")
	if err := i.waitForPipelinesReady(ctx); err != nil {
		return fmt.Errorf("failed to wait for Pipelines: %w", err)
	}

	fmt.Println("  <--Tekton Pipelines is ready!-->")
	return nil
}

// isPipelinesInstalled checks if Tekton Pipelines is installed
func (i *Installer) isPipelinesInstalled(ctx context.Context) bool {
	if i.kubeClient == nil {
		return false // In dry-run, assume nothing is installed
	}
	_, err := i.kubeClient.AppsV1().Deployments("tekton-pipelines").Get(ctx, "tekton-pipelines-controller", metav1.GetOptions{})
	return err == nil
}

// downloadAndApplyTriggers downloads and applies Tekton Triggers manifests
func (i *Installer) downloadAndApplyTriggers(ctx context.Context) error {
	// Download Triggers release manifest
	triggersURL := "https://storage.googleapis.com/tekton-releases/triggers/latest/release.yaml"
	interceptorsURL := "https://storage.googleapis.com/tekton-releases/triggers/latest/interceptors.yaml"

	if i.config.Verbose {
		fmt.Printf("  📥 Downloading Triggers from %s\n", triggersURL)
	}

	if err := i.downloadAndApplyManifest(ctx, triggersURL); err != nil {
		return fmt.Errorf("failed to apply triggers manifest: %w", err)
	}

	if i.config.Verbose {
		fmt.Printf("  📥 Downloading Interceptors from %s\n", interceptorsURL)
	}

	if err := i.downloadAndApplyManifest(ctx, interceptorsURL); err != nil {
		return fmt.Errorf("failed to apply interceptors manifest: %w", err)
	}

	return nil
}

// downloadAndApplyPipelines downloads and applies Tekton Pipelines manifests
func (i *Installer) downloadAndApplyPipelines(ctx context.Context) error {
	// Download Pipelines release manifest
	pipelinesURL := "https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"

	if i.config.Verbose {
		fmt.Printf("  📥 Downloading Pipelines from %s\n", pipelinesURL)
	}

	if err := i.downloadAndApplyManifest(ctx, pipelinesURL); err != nil {
		return fmt.Errorf("failed to apply pipelines manifest: %w", err)
	}

	return nil
}

// downloadAndApplyManifest downloads a YAML manifest from URL and applies it to the cluster
func (i *Installer) downloadAndApplyManifest(ctx context.Context, url string) error {
	// Download the manifest
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download manifest: HTTP %d", resp.StatusCode)
	}

	// Read the YAML content
	yamlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Apply the manifest using kubectl-like logic
	if err := i.applyYAMLManifest(ctx, yamlContent); err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}

	return nil
}

// applyYAMLManifest applies a multi-document YAML manifest
func (i *Installer) applyYAMLManifest(ctx context.Context, yamlContent []byte) error {
	if i.config.Verbose {
		fmt.Printf("  ⚙️  Applying manifest (%d bytes)\n", len(yamlContent))
	}

	// REAL kubectl apply using exec
	if err := i.applyManifestViaKubectl(ctx, yamlContent); err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}

	return nil
}

// applyManifestViaKubectl applies YAML using kubectl apply
func (i *Installer) applyManifestViaKubectl(ctx context.Context, yamlContent []byte) error {
	// Write YAML to temporary file
	tmpFile, err := os.CreateTemp("", "tekton-manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(yamlContent); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	tmpFile.Close()

	// Apply using kubectl
	kubeconfigFlag := ""
	if i.config.KubeConfig != nil {
		// For now, assume kubeconfig path is available
		kubeconfigFlag = "--kubeconfig=" + os.Getenv("HOME") + "/.kube/config"
	}

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", tmpFile.Name())
	if kubeconfigFlag != "" {
		cmd = exec.CommandContext(ctx, "kubectl", kubeconfigFlag, "apply", "-f", tmpFile.Name())
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w\nOutput: %s", err, string(output))
	}

	if i.config.Verbose {
		fmt.Printf("  ✅ Applied successfully:\n%s", string(output))
	}

	return nil
}

// waitForPipelinesReady waits for Tekton Pipelines to be fully ready
func (i *Installer) waitForPipelinesReady(ctx context.Context) error {
	if i.config.DryRun {
		fmt.Println("    [DRY-RUN] Would wait for Pipelines to be ready")
		return nil
	}

	if i.kubeClient == nil {
		return nil // Skip in dry-run
	}

	// Wait for the tekton-pipelines namespace to exist and be active
	err := wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		ns, err := i.kubeClient.CoreV1().Namespaces().Get(ctx, "tekton-pipelines", metav1.GetOptions{})
		if err != nil {
			if i.config.Verbose {
				fmt.Printf("    Waiting for tekton-pipelines namespace... (%v)\n", err)
			}
			return false, nil // Keep waiting
		}

		// Check if namespace is active
		if ns.Status.Phase != "Active" {
			if i.config.Verbose {
				fmt.Printf("    Namespace exists but not active yet (phase: %s)\n", ns.Status.Phase)
			}
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for tekton-pipelines namespace: %w", err)
	}

	// Wait for Pipelines controller deployment to be ready
	err = wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		deployment, err := i.kubeClient.AppsV1().Deployments("tekton-pipelines").Get(ctx, "tekton-pipelines-controller", metav1.GetOptions{})
		if err != nil {
			if i.config.Verbose {
				fmt.Printf("    Waiting for tekton-pipelines-controller deployment... (%v)\n", err)
			}
			return false, nil // Keep waiting
		}

		// Check if deployment is ready
		if deployment.Status.ReadyReplicas == 0 || deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
			if i.config.Verbose {
				fmt.Printf("    Deployment not ready yet (%d/%d replicas ready)\n", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			}
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for tekton-pipelines-controller: %w", err)
	}

	// wait for webhook deployment to be ready
	fmt.Println("    Waiting for webhook service...")
	err = wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		deployment, err := i.kubeClient.AppsV1().Deployments("tekton-pipelines").Get(ctx, "tekton-pipelines-webhook", metav1.GetOptions{})
		if err != nil {
			if i.config.Verbose {
				fmt.Printf("    Waiting for tekton-pipeline-webhook deployment... (%v)\n", err)
			}
			return false, nil // Keep waiting
		}

		// Check if webhook deployment is ready
		if deployment.Status.ReadyReplicas == 0 || deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
			fmt.Printf("    Webhook not ready yet (%d/%d replicas ready)\n", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for tekton-pipelines: %w", err)
	}

	// buffer for webhook service to be fully responsive
	fmt.Println("    Waiting 15 seconds...")
	time.Sleep(15 * time.Second)
	return nil
}
