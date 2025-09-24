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

	triggersv1beta1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1beta1"
	triggersclientset "github.com/tektoncd/triggers/pkg/client/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TemplatesManager handles creation of Trigger templates and resources
type TemplatesManager struct {
	triggersClient triggersclientset.Interface
	config         *Config
}

// NewTemplatesManager creates a new templates manager
func NewTemplatesManager(triggersClient triggersclientset.Interface, config *Config) *TemplatesManager {
	return &TemplatesManager{
		triggersClient: triggersClient,
		config:         config,
	}
}

// CreateTriggerResources creates the exact Trigger resources from getting-started docs
func (t *TemplatesManager) CreateTriggerResources(ctx context.Context) error {
	// Apply the EXACT triggers.yaml from docs/getting-started as per README
	return t.applyGettingStartedTriggers(ctx)
}

func (t *TemplatesManager) createTriggerBinding(ctx context.Context) error {
	if t.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create TriggerBinding: bootstrap-binding\n")
		return nil
	}

	binding := &triggersv1beta1.TriggerBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-binding",
			Namespace: t.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Spec: triggersv1beta1.TriggerBindingSpec{
			Params: []triggersv1beta1.Param{
				{
					Name:  "git-revision",
					Value: t.getRevisionValue(),
				},
				{
					Name:  "git-url",
					Value: t.getGitURLValue(),
				},
				{
					Name:  "git-repo-name",
					Value: t.getRepoNameValue(),
				},
			},
		},
	}

	_, err := t.triggersClient.TriggersV1beta1().TriggerBindings(t.config.Namespace).Create(ctx, binding, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if t.config.Verbose {
			fmt.Printf("TriggerBinding 'bootstrap-binding' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create trigger binding: %w", err)
	}

	if t.config.Verbose {
		fmt.Printf("Created TriggerBinding: bootstrap-binding\n")
	}

	return nil
}

func (t *TemplatesManager) createTriggerTemplate(ctx context.Context) error {
	if t.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create TriggerTemplate: bootstrap-template\n")
		return nil
	}

	template := &triggersv1beta1.TriggerTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-template",
			Namespace: t.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Spec: triggersv1beta1.TriggerTemplateSpec{
			Params: []triggersv1beta1.ParamSpec{
				{
					Name:        "git-revision",
					Description: "The git revision",
					Default:     stringPtr("main"),
				},
				{
					Name:        "git-url",
					Description: "The git repository URL",
				},
				{
					Name:        "git-repo-name",
					Description: "The git repository name",
				},
			},
			ResourceTemplates: []triggersv1beta1.TriggerResourceTemplate{
				{
					RawExtension: runtime.RawExtension{
						Raw: t.getPipelineRunTemplate(),
					},
				},
			},
		},
	}

	_, err := t.triggersClient.TriggersV1beta1().TriggerTemplates(t.config.Namespace).Create(ctx, template, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if t.config.Verbose {
			fmt.Printf("TriggerTemplate 'bootstrap-template' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create trigger template: %w", err)
	}

	if t.config.Verbose {
		fmt.Printf("Created TriggerTemplate: bootstrap-template\n")
	}

	return nil
}

func (t *TemplatesManager) createEventListener(ctx context.Context) error {
	if t.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would create EventListener: bootstrap-listener\n")
		return nil
	}

	listener := &triggersv1beta1.EventListener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-listener",
			Namespace: t.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "tekton-triggers",
				"tekton.dev/bootstrap":      "true",
			},
		},
		Spec: triggersv1beta1.EventListenerSpec{
			ServiceAccountName: "tekton-triggers-example-sa",
			Triggers: []triggersv1beta1.EventListenerTrigger{
				{
					Name: "bootstrap-trigger",
					Bindings: []*triggersv1beta1.EventListenerBinding{
						{
							Ref: "bootstrap-binding",
						},
					},
					Template: &triggersv1beta1.EventListenerTemplate{
						Ref: stringPtr("bootstrap-template"),
					},
				},
			},
		},
	}

	_, err := t.triggersClient.TriggersV1beta1().EventListeners(t.config.Namespace).Create(ctx, listener, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if t.config.Verbose {
			fmt.Printf("EventListener 'bootstrap-listener' already exists\n")
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create event listener: %w", err)
	}

	if t.config.Verbose {
		fmt.Printf("Created EventListener: bootstrap-listener\n")
	}

	return nil
}

// CreateExamples creates the exact Pipeline from getting-started docs
func (t *TemplatesManager) CreateExamples(ctx context.Context) error {
	// Apply the EXACT pipeline.yaml from docs/getting-started as per README
	return t.applyGettingStartedPipeline(ctx)
}

// applyGettingStartedTriggers applies the exact triggers.yaml from getting-started docs
func (t *TemplatesManager) applyGettingStartedTriggers(ctx context.Context) error {
	if t.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would apply docs/getting-started/triggers.yaml\n")
		return nil
	}

	if err := t.applyTriggersFile(ctx, "docs/getting-started/triggers.yaml"); err != nil {
		return fmt.Errorf("failed to apply triggers.yaml: %w", err)
	}

	if t.config.Verbose {
		fmt.Printf("  ✅ Applied getting-started triggers.yaml\n")
	}

	return nil
}

// applyGettingStartedPipeline applies the exact pipeline.yaml from getting-started docs
func (t *TemplatesManager) applyGettingStartedPipeline(ctx context.Context) error {
	if t.config.DryRun {
		fmt.Printf("  [DRY-RUN] Would apply docs/getting-started/pipeline.yaml\n")
		return nil
	}

	if err := t.applyTriggersFile(ctx, "docs/getting-started/pipeline.yaml"); err != nil {
		return fmt.Errorf("failed to apply pipeline.yaml: %w", err)
	}

	if t.config.Verbose {
		fmt.Printf("  ✅ Applied getting-started pipeline.yaml\n")
	}

	return nil
}

// applyTriggersFile applies a YAML file using kubectl apply
func (t *TemplatesManager) applyTriggersFile(ctx context.Context, filePath string) error {
	kubeconfigFlag := ""
	if t.config.KubeConfig != nil {
		kubeconfigFlag = "--kubeconfig=" + os.Getenv("HOME") + "/.kube/config"
	}

	var cmd *exec.Cmd
	if kubeconfigFlag != "" {
		cmd = exec.CommandContext(ctx, "kubectl", kubeconfigFlag, "-n", t.config.Namespace, "apply", "-f", filePath)
	} else {
		cmd = exec.CommandContext(ctx, "kubectl", "-n", t.config.Namespace, "apply", "-f", filePath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed for %s: %w\nOutput: %s", filePath, err, string(output))
	}

	if t.config.Verbose {
		fmt.Printf("  Applied %s:\n%s", filePath, string(output))
	}

	return nil
}

// Helper functions for generating provider-specific values
func (t *TemplatesManager) getRevisionValue() string {
	switch t.config.Provider {
	case "github":
		return "$(body.head_commit.id)"
	case "gitlab":
		return "$(body.checkout_sha)"
	case "bitbucket":
		return "$(body.push.changes[0].new.target.hash)"
	default:
		return "$(body.head_commit.id)" // Default to GitHub format
	}
}

func (t *TemplatesManager) getGitURLValue() string {
	switch t.config.Provider {
	case "github":
		return "$(body.repository.clone_url)"
	case "gitlab":
		return "$(body.repository.git_http_url)"
	case "bitbucket":
		return "$(body.repository.links.clone[0].href)"
	default:
		return "$(body.repository.clone_url)" // Default to GitHub format
	}
}

func (t *TemplatesManager) getRepoNameValue() string {
	switch t.config.Provider {
	case "github":
		return "$(body.repository.name)"
	case "gitlab":
		return "$(body.repository.name)"
	case "bitbucket":
		return "$(body.repository.name)"
	default:
		return "$(body.repository.name)"
	}
}

// getPipelineRunTemplate returns a basic PipelineRun template as JSON
func (t *TemplatesManager) getPipelineRunTemplate() []byte {
	// This would be a proper JSON representation of a PipelineRun
	// For demo purposes, this is simplified
	template := `{
		"apiVersion": "tekton.dev/v1beta1",
		"kind": "PipelineRun",
		"metadata": {
			"generateName": "bootstrap-run-",
			"namespace": "` + t.config.Namespace + `"
		},
		"spec": {
			"serviceAccountName": "triggers-bootstrap-sa",
			"pipelineRef": {
				"name": "bootstrap-pipeline"
			},
			"params": [
				{
					"name": "git-url",
					"value": "$(tt.params.git-url)"
				},
				{
					"name": "git-revision",
					"value": "$(tt.params.git-revision)"
				}
			],
			"workspaces": [
				{
					"name": "git-source",
					"volumeClaimTemplate": {
						"spec": {
							"accessModes": ["ReadWriteOnce"],
							"resources": {
								"requests": {
									"storage": "1Gi"
								}
							}
						}
					}
				}
			]
		}
	}`

	return []byte(template)
}

// Helper function to convert string to string pointer
func stringPtr(s string) *string {
	return &s
}
