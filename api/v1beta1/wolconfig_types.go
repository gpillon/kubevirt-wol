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

package v1beta1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscoveryMode defines how VMs are discovered for WOL management
// +kubebuilder:validation:Enum=All;LabelSelector;Explicit
type DiscoveryMode string

const (
	// DiscoveryModeAll watches all VMs in selected namespaces
	DiscoveryModeAll DiscoveryMode = "All"
	// DiscoveryModeLabelSelector watches VMs matching label selector
	DiscoveryModeLabelSelector DiscoveryMode = "LabelSelector"
	// DiscoveryModeExplicit uses explicit MAC to VM mappings
	DiscoveryModeExplicit DiscoveryMode = "Explicit"
)

// MACVMMapping defines an explicit MAC address to VM mapping
type MACVMMapping struct {
	// MACAddress in format xx:xx:xx:xx:xx:xx
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`
	MACAddress string `json:"macAddress"`
	// VMName is the name of the VirtualMachine
	VMName string `json:"vmName"`
	// Namespace where the VM resides
	Namespace string `json:"namespace"`
}

// WolConfigSpec defines the desired state of WolConfig
type WolConfigSpec struct {
	// DiscoveryMode determines how VMs are discovered
	// +kubebuilder:default=All
	// +optional
	DiscoveryMode DiscoveryMode `json:"discoveryMode,omitempty"`

	// NamespaceSelectors lists namespaces to watch for VMs
	// If empty, all namespaces are monitored
	// +optional
	NamespaceSelectors []string `json:"namespaceSelectors,omitempty"`

	// VMSelector is a label selector for VMs (used with DiscoveryMode=LabelSelector)
	// +optional
	VMSelector *metav1.LabelSelector `json:"vmSelector,omitempty"`

	// ExplicitMappings provides explicit MAC to VM mappings (used with DiscoveryMode=Explicit)
	// +optional
	ExplicitMappings []MACVMMapping `json:"explicitMappings,omitempty"`

	// WOLPorts are the UDP ports to listen for Wake-on-LAN packets
	// Default: [9]
	// +kubebuilder:default={9}
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	WOLPorts []int `json:"wolPorts,omitempty"`

	// CacheTTL is the cache time-to-live in seconds for VM mappings
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=0
	// +optional
	CacheTTL int `json:"cacheTTL,omitempty"`

	// Agent configuration for the WOL DaemonSet
	// +optional
	Agent AgentSpec `json:"agent,omitempty"`
}

// AgentSpec defines the DaemonSet configuration for WOL agents
type AgentSpec struct {
	// NodeSelector is a selector which must be true for the agent pod to fit on a node
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations allow the agent pods to schedule onto nodes with matching taints
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Resources describes the compute resource requirements for agent pods
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Image is the container image for the agent (optional, defaults to controller's agent image)
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy for agent container image
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// UpdateStrategy for the DaemonSet
	// +optional
	UpdateStrategy *appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// PriorityClassName for agent pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// WolConfigStatus defines the observed state of WolConfig
type WolConfigStatus struct {
	// ManagedVMs is the number of VMs currently being monitored
	// +optional
	ManagedVMs int `json:"managedVMs,omitempty"`

	// LastSync is the timestamp of the last VM mapping update
	// +optional
	LastSync *metav1.Time `json:"lastSync,omitempty"`

	// Conditions represent the latest available observations of the WOLConfig state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AgentStatus contains information about the agent DaemonSet
	// +optional
	AgentStatus *AgentStatus `json:"agentStatus,omitempty"`
}

// AgentStatus contains status information about the agent DaemonSet
type AgentStatus struct {
	// DaemonSetName is the name of the created DaemonSet
	DaemonSetName string `json:"daemonSetName,omitempty"`

	// DesiredNumberScheduled is the total number of nodes that should be running the daemon pod
	DesiredNumberScheduled int32 `json:"desiredNumberScheduled,omitempty"`

	// NumberReady is the number of nodes with ready daemon pods
	NumberReady int32 `json:"numberReady,omitempty"`

	// NumberAvailable is the number of nodes with available daemon pods
	NumberAvailable int32 `json:"numberAvailable,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=wolcfg
// +kubebuilder:printcolumn:name="Discovery Mode",type=string,JSONPath=`.spec.discoveryMode`
// +kubebuilder:printcolumn:name="WOL Port",type=integer,JSONPath=`.spec.wolPort`
// +kubebuilder:printcolumn:name="Managed VMs",type=integer,JSONPath=`.status.managedVMs`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WolConfig is the Schema for the Wake-on-LAN configurations API
type WolConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WolConfigSpec   `json:"spec,omitempty"`
	Status WolConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WolConfigList contains a list of WolConfig
type WolConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WolConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WolConfig{}, &WolConfigList{})
}
