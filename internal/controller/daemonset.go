/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

const (
	DefaultAgentImage       = "quay.io/kubevirtwol/kubevirt-wol-agent:v2-142900"
	DefaultOperatorAddress  = "kubevirt-wol-kubevirt-wol-grpc.kubevirt-wol-system.svc:9090"
	AgentNamespace          = "kubevirt-wol-system"
	AgentServiceAccountName = "kubevirt-wol-wol-agent" // Must match the ServiceAccount created by kustomize (with namePrefix)
)

// reconcileAgentDaemonSet creates or updates the agent DaemonSet for the given WolConfig
func (r *WolConfigReconciler) reconcileAgentDaemonSet(ctx context.Context, wolConfig *wolv1beta1.WolConfig) error {
	log := ctrl.LoggerFrom(ctx)
	daemonSetName := getDaemonSetName(wolConfig)

	// Build desired DaemonSet
	desiredDS := r.buildAgentDaemonSet(wolConfig, daemonSetName)

	// Set owner reference
	if err := controllerutil.SetControllerReference(wolConfig, desiredDS, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Check if DaemonSet already exists
	existingDS := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      daemonSetName,
		Namespace: AgentNamespace,
	}, existingDS)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new DaemonSet
			log.Info("Creating agent DaemonSet", "name", daemonSetName, "wolconfig", wolConfig.Name)
			if err := r.Create(ctx, desiredDS); err != nil {
				return fmt.Errorf("failed to create DaemonSet: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get DaemonSet: %w", err)
	}

	// Update existing DaemonSet
	log.Info("Updating agent DaemonSet", "name", daemonSetName, "wolconfig", wolConfig.Name)
	existingDS.Spec = desiredDS.Spec
	if err := r.Update(ctx, existingDS); err != nil {
		return fmt.Errorf("failed to update DaemonSet: %w", err)
	}

	return nil
}

// buildAgentDaemonSet constructs the DaemonSet spec for the agent
func (r *WolConfigReconciler) buildAgentDaemonSet(wolConfig *wolv1beta1.WolConfig, name string) *appsv1.DaemonSet {
	labels := map[string]string{
		"app":                          "wol-agent",
		"app.kubernetes.io/name":       "wol-agent",
		"app.kubernetes.io/component":  "agent",
		"app.kubernetes.io/part-of":    "kubevirt-wol",
		"app.kubernetes.io/managed-by": "kubevirt-wol-controller",
		"wol.pillon.org/wolconfig":     wolConfig.Name,
	}

	// Determine image
	image := DefaultAgentImage
	if wolConfig.Spec.Agent.Image != "" {
		image = wolConfig.Spec.Agent.Image
	}

	imagePullPolicy := corev1.PullAlways // Always pull to get latest updates
	if wolConfig.Spec.Agent.ImagePullPolicy != "" {
		imagePullPolicy = wolConfig.Spec.Agent.ImagePullPolicy
	}

	// Build ports env var (comma-separated)
	ports := wolConfig.Spec.WOLPorts
	if len(ports) == 0 {
		ports = []int{9} // Default
	}
	portsStr := make([]string, len(ports))
	for i, p := range ports {
		portsStr[i] = fmt.Sprintf("%d", p)
	}

	// Build container
	container := corev1.Container{
		Name:            "agent",
		Image:           image,
		ImagePullPolicy: imagePullPolicy,
		Args: []string{
			"--node-name=$(NODE_NAME)",
			"--operator-address=" + DefaultOperatorAddress,
			"--ports=" + strings.Join(portsStr, ","),
			"--zap-log-level=info",
		},
		Env: []corev1.EnvVar{
			{
				Name: "NODE_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "WOLCONFIG_NAME",
				Value: wolConfig.Name,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                pointer(int64(0)),
			AllowPrivilegeEscalation: pointer(false),
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{"NET_BIND_SERVICE"},
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Resources: wolConfig.Spec.Agent.Resources,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz",
					Port:   intstr.FromInt(8080),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/readyz",
					Port:   intstr.FromInt(8080),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      3,
			FailureThreshold:    3,
		},
	}

	// Default resources if not specified
	if container.Resources.Requests == nil && container.Resources.Limits == nil {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}
	}

	// Build pod spec
	podSpec := corev1.PodSpec{
		HostNetwork:        true,
		DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
		ServiceAccountName: AgentServiceAccountName,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: pointer(int64(0)),
		},
		Containers: []corev1.Container{container},
	}

	// Apply node selector if specified
	if len(wolConfig.Spec.Agent.NodeSelector) > 0 {
		podSpec.NodeSelector = wolConfig.Spec.Agent.NodeSelector
	}

	// Apply tolerations if specified
	if len(wolConfig.Spec.Agent.Tolerations) > 0 {
		podSpec.Tolerations = wolConfig.Spec.Agent.Tolerations
	} else {
		// Default tolerations
		podSpec.Tolerations = []corev1.Toleration{
			{
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpExists,
			},
			{
				Effect:   corev1.TaintEffectNoExecute,
				Operator: corev1.TolerationOpExists,
			},
		}
	}

	// Apply priority class if specified
	if wolConfig.Spec.Agent.PriorityClassName != "" {
		podSpec.PriorityClassName = wolConfig.Spec.Agent.PriorityClassName
	}

	// Build update strategy
	updateStrategy := appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: pointer(intstr.FromInt(1)),
		},
	}
	if wolConfig.Spec.Agent.UpdateStrategy != nil {
		updateStrategy = *wolConfig.Spec.Agent.UpdateStrategy
	}

	// Build DaemonSet
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: AgentNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: updateStrategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	return ds
}

// getDaemonSetName returns the name of the DaemonSet for the given WolConfig
func getDaemonSetName(wolConfig *wolv1beta1.WolConfig) string {
	return fmt.Sprintf("wol-agent-%s", wolConfig.Name)
}

// pointer is a helper to get pointer to a value
func pointer[T any](v T) *T {
	return &v
}
