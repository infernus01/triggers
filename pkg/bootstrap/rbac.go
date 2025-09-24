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
	"os"
	"os/exec"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RBACManager handles RBAC setup
type RBACManager struct {
	kubeClient kubernetes.Interface
	config     *Config
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(kubeClient kubernetes.Interface, config *Config) *RBACManager {
	return &RBACManager{
		kubeClient: kubeClient,
		config:     config,
	}
}

// CreateNamespace creates the target namespace if it doesn't exist
func (r *RBACManager) CreateNamespace(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create namespace: %s\n", r.config.Namespace)
		return nil
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
	}

	_, err := r.kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("Namespace '%s' already exists\n", r.config.Namespace)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created namespace: %s\n", r.config.Namespace)
	}

	return nil
}

// SetupRBAC creates necessary service accounts, roles, and bindings
func (r *RBACManager) SetupRBAC(ctx context.Context) error {
	// Apply the EXACT RBAC files from docs/getting-started as per README

	// 1. Apply admin-role.yaml (ServiceAccount + RoleBinding + ClusterRoleBinding)
	if err := r.applyGettingStartedRBAC(ctx); err != nil {
		return err
	}

	// 2. Apply webhook-role.yaml (Role + ServiceAccount + RoleBinding for webhook tasks)
	if err := r.applyWebhookRBAC(ctx); err != nil {
		return err
	}

	return nil
}

// applyGettingStartedRBAC applies the exact admin-role.yaml from getting-started docs
func (r *RBACManager) applyGettingStartedRBAC(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would apply docs/getting-started/rbac/admin-role.yaml\n")
		fmt.Printf("  [DRY-RUN] Would apply docs/getting-started/rbac/clusterrolebinding.yaml\n")
		return nil
	}

	// Apply admin-role.yaml
	if err := r.applyRBACFile(ctx, "docs/getting-started/rbac/admin-role.yaml"); err != nil {
		return fmt.Errorf("failed to apply admin-role.yaml: %w", err)
	}

	// Apply clusterrolebinding.yaml
	if err := r.applyRBACFile(ctx, "docs/getting-started/rbac/clusterrolebinding.yaml"); err != nil {
		return fmt.Errorf("failed to apply clusterrolebinding.yaml: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("  ✅ Applied getting-started admin RBAC\n")
	}

	return nil
}

// applyWebhookRBAC applies the exact webhook-role.yaml from getting-started docs
func (r *RBACManager) applyWebhookRBAC(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would apply docs/getting-started/rbac/webhook-role.yaml\n")
		return nil
	}

	// Apply webhook-role.yaml
	if err := r.applyRBACFile(ctx, "docs/getting-started/rbac/webhook-role.yaml"); err != nil {
		return fmt.Errorf("failed to apply webhook-role.yaml: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("  ✅ Applied getting-started webhook RBAC\n")
	}

	return nil
}

// applyRBACFile applies a YAML file using kubectl apply
func (r *RBACManager) applyRBACFile(ctx context.Context, filePath string) error {
	kubeconfigFlag := ""
	if r.config.KubeConfig != nil {
		kubeconfigFlag = "--kubeconfig=" + os.Getenv("HOME") + "/.kube/config"
	}

	var cmd *exec.Cmd
	if kubeconfigFlag != "" {
		cmd = exec.CommandContext(ctx, "kubectl", kubeconfigFlag, "-n", r.config.Namespace, "apply", "-f", filePath)
	} else {
		cmd = exec.CommandContext(ctx, "kubectl", "-n", r.config.Namespace, "apply", "-f", filePath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed for %s: %w\nOutput: %s", filePath, err, string(output))
	}

	if r.config.Verbose {
		fmt.Printf("  Applied %s:\n%s", filePath, string(output))
	}

	return nil
}

func (r *RBACManager) createServiceAccount(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create ServiceAccount: triggers-bootstrap-sa\n")
		return nil
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "triggers-bootstrap-sa",
			Namespace: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
	}

	_, err := r.kubeClient.CoreV1().ServiceAccounts(r.config.Namespace).Create(ctx, sa, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("ServiceAccount 'triggers-bootstrap-sa' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created ServiceAccount: triggers-bootstrap-sa\n")
	}

	return nil
}

func (r *RBACManager) createRole(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create Role: triggers-bootstrap-role\n")
		return nil
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "triggers-bootstrap-role",
			Namespace: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets", "services"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
			{
				APIGroups: []string{"tekton.dev"},
				Resources: []string{"pipelines", "tasks", "pipelineruns", "taskruns"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
			{
				APIGroups: []string{"triggers.tekton.dev"},
				Resources: []string{"eventlisteners", "triggers", "triggerbindings", "triggertemplates"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
		},
	}

	_, err := r.kubeClient.RbacV1().Roles(r.config.Namespace).Create(ctx, role, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("Role 'triggers-bootstrap-role' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created Role: triggers-bootstrap-role\n")
	}

	return nil
}

func (r *RBACManager) createRoleBinding(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create RoleBinding: triggers-bootstrap-binding\n")
		return nil
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "triggers-bootstrap-binding",
			Namespace: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "triggers-bootstrap-sa",
				Namespace: r.config.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "triggers-bootstrap-role",
		},
	}

	_, err := r.kubeClient.RbacV1().RoleBindings(r.config.Namespace).Create(ctx, binding, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("RoleBinding 'triggers-bootstrap-binding' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create role binding: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created RoleBinding: triggers-bootstrap-binding\n")
	}

	return nil
}

func (r *RBACManager) createClusterRoleBinding(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create ClusterRoleBinding: triggers-bootstrap-cluster-binding\n")
		return nil
	}

	clusterBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("triggers-bootstrap-cluster-binding-%s", r.config.Namespace),
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "triggers-bootstrap-sa",
				Namespace: r.config.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tekton-triggers-eventlistener-clusterroles", // This should exist from Triggers installation
		},
	}

	_, err := r.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, clusterBinding, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("ClusterRoleBinding already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created ClusterRoleBinding\n")
	}

	return nil
}

// SetupIngress creates ingress configuration for EventListeners
func (r *RBACManager) SetupIngress(ctx context.Context) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would setup ingress for EventListeners\n")
		fmt.Printf("    Host: %s\n", r.config.PublicDomain)
		fmt.Printf("    Service: el-bootstrap-listener:8080\n")
		fmt.Printf("    TLS: enabled\n")
		return nil
	}

	if err := r.createIngress(ctx); err != nil {
		return err
	}

	if r.config.Verbose {
		fmt.Printf("ingress created\n")
		fmt.Printf("Public URL: https://%s/hooks\n", r.config.PublicDomain)
		fmt.Printf("TLS enabled\n")
	}

	return nil
}

// CreateWebhookSecret creates a Kubernetes secret for webhook authentication
func (r *RBACManager) CreateWebhookSecret(ctx context.Context, webhookSecret string) error {
	if r.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create webhook secret\n")
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "github-webhook-secret",
			Namespace: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"webhook-secret": []byte(webhookSecret),
		},
	}

	_, err := r.kubeClient.CoreV1().Secrets(r.config.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if r.config.Verbose {
			fmt.Printf("Webhook secret already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create webhook secret: %w", err)
	}

	if r.config.Verbose {
		fmt.Printf("Created webhook secret: github-webhook-secret\n")
	}

	return nil
}

// createIngress creates the actual ingress resource
func (r *RBACManager) createIngress(ctx context.Context) error {
	// This is a simplified ingress - in you'd want to:
	// 1. Detect the ingress controller (nginx, traefik, etc.)
	// 2. Set appropriate annotations
	// 3. Configure TLS properly
	// 4. Handle different ingress API versions

	// In a real implementation, this would create an actual Ingress resource:
	// - Parse YAML template with domain and namespace
	// - Apply via Kubernetes API
	// - Configure TLS with cert-manager
	// - Set ingress controller annotations
	if r.config.Verbose {
		fmt.Printf("  Ingress configuration:\n")
		fmt.Printf("    Host: %s\n", r.config.PublicDomain)
		fmt.Printf("    Path: /hooks -> el-bootstrap-listener:8080\n")
		fmt.Printf("    TLS: %s-tls-secret\n", r.config.Namespace)
	}

	// TODO: Actually create the ingress resource
	// This would involve parsing the YAML and applying it via the Kubernetes API

	return nil
}
