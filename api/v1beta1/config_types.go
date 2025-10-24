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

// ConfigSpec defines the desired state of WOLConfig
type ConfigSpec struct {
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

	// WOLPort is the UDP port to listen for Wake-on-LAN packets
	// +kubebuilder:default=9
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	WOLPort int `json:"wolPort,omitempty"`

	// CacheTTL is the cache time-to-live in seconds for VM mappings
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=0
	// +optional
	CacheTTL int `json:"cacheTTL,omitempty"`
}

// ConfigStatus defines the observed state of WOLConfig
type ConfigStatus struct {
	// ManagedVMs is the number of VMs currently being monitored
	// +optional
	ManagedVMs int `json:"managedVMs,omitempty"`

	// LastSync is the timestamp of the last VM mapping update
	// +optional
	LastSync *metav1.Time `json:"lastSync,omitempty"`

	// Conditions represent the latest available observations of the WOLConfig state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Discovery Mode",type=string,JSONPath=`.spec.discoveryMode`
// +kubebuilder:printcolumn:name="WOL Port",type=integer,JSONPath=`.spec.wolPort`
// +kubebuilder:printcolumn:name="Managed VMs",type=integer,JSONPath=`.status.managedVMs`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Config is the Schema for the Wake-on-LAN configurations API
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigList contains a list of WOLConfig
type ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Config `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Config{}, &ConfigList{})
}
