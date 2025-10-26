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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
	"github.com/gpillon/kubevirt-wol/internal/wol"
)

const (
	// ConditionTypeReady indicates the WOLConfig is ready
	ConditionTypeReady = "Ready"
	// ReasonConfigured indicates configuration is valid
	ReasonConfigured = "Configured"
	// ReasonInvalidConfig indicates configuration is invalid
	ReasonInvalidConfig = "InvalidConfig"
	// ReasonMappingUpdated indicates mapping was successfully updated
	ReasonMappingUpdated = "MappingUpdated"
	// ReasonAgentFailed indicates agent DaemonSet reconciliation failed
	ReasonAgentFailed = "AgentFailed"
)

// WolConfigReconciler reconciles a WolConfig object
type WolConfigReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Mapper            *wol.MACMapper
	VMStarter         *wol.VMStarter
	AgentImage        string // Agent image to use for DaemonSets (from AGENT_IMAGE env var)
	OperatorNamespace string // Namespace where operator is running (from POD_NAMESPACE env var)
}

// +kubebuilder:rbac:groups=wol.pillon.org,resources=wolconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=wol.pillon.org,resources=wolconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=wol.pillon.org,resources=wolconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=subresources.kubevirt.io,resources=virtualmachines/start,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets/status,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile handles WolConfig reconciliation
func (r *WolConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WolConfig instance
	config := &wolv1beta1.WolConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if errors.IsNotFound(err) {
			// Config deleted, nothing to do
			logger.Info("WolConfig deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get WolConfig")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling WolConfig",
		"name", config.Name,
		"discoveryMode", config.Spec.DiscoveryMode,
		"wolPorts", config.Spec.WOLPorts)

	// Validate configuration
	if err := r.validateConfig(config); err != nil {
		logger.Error(err, "Invalid configuration")
		if statusErr := r.updateStatus(ctx, config, false, ReasonInvalidConfig, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// Reconcile agent DaemonSet
	if err := r.reconcileAgentDaemonSet(ctx, config); err != nil {
		logger.Error(err, "Failed to reconcile agent DaemonSet")
		if statusErr := r.updateStatus(ctx, config, false, ReasonAgentFailed, fmt.Sprintf("Failed to reconcile DaemonSet: %v", err)); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Refresh global mapping from ALL WOLConfigs (not just this one)
	// This ensures multiple configs work in OR mode, not AND
	managedVMs, err := r.refreshAllConfigs(ctx)
	if err != nil {
		logger.Error(err, "Failed to refresh VM mapping from all configs")
		if statusErr := r.updateStatus(ctx, config, false, ReasonInvalidConfig, fmt.Sprintf("Failed to refresh mapping: %v", err)); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status for this specific config
	now := metav1.Now()
	config.Status.ManagedVMs = managedVMs
	config.Status.LastSync = &now

	// Update agent status from DaemonSet
	if err := r.updateAgentStatus(ctx, config); err != nil {
		logger.Error(err, "Failed to update agent status")
		// Non fatal, continua
	}

	if err := r.updateStatus(ctx, config, true, ReasonMappingUpdated, "VM mapping refreshed successfully"); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled WolConfig",
		"managedVMs", config.Status.ManagedVMs,
		"lastSync", config.Status.LastSync)

	// Requeue to refresh mapping periodically
	requeueAfter := time.Duration(config.Spec.CacheTTL) * time.Second
	if requeueAfter == 0 {
		requeueAfter = 5 * time.Minute
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// validateConfig validates the WolConfig specification
func (r *WolConfigReconciler) validateConfig(config *wolv1beta1.WolConfig) error {
	// Validate discovery mode
	if config.Spec.DiscoveryMode == "" {
		config.Spec.DiscoveryMode = wolv1beta1.DiscoveryModeAll
	}

	// Validate WOL ports
	if len(config.Spec.WOLPorts) == 0 {
		config.Spec.WOLPorts = []int{9} // Default
	}
	for _, port := range config.Spec.WOLPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid WOL port: %d (must be 1-65535)", port)
		}
	}

	// Validate cache TTL
	if config.Spec.CacheTTL == 0 {
		config.Spec.CacheTTL = 300
	}
	if config.Spec.CacheTTL < 0 {
		return fmt.Errorf("invalid cache TTL: %d (must be >= 0)", config.Spec.CacheTTL)
	}

	// Validate based on discovery mode
	switch config.Spec.DiscoveryMode {
	case wolv1beta1.DiscoveryModeLabelSelector:
		if config.Spec.VMSelector == nil {
			return fmt.Errorf("VMSelector is required for LabelSelector discovery mode")
		}
	case wolv1beta1.DiscoveryModeExplicit:
		if len(config.Spec.ExplicitMappings) == 0 {
			return fmt.Errorf("ExplicitMappings is required for Explicit discovery mode")
		}
	}

	return nil
}

// updateStatus updates the WolConfig status
func (r *WolConfigReconciler) updateStatus(ctx context.Context, config *wolv1beta1.WolConfig, ready bool, reason, message string) error {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	// Update or add Ready condition
	condition := metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             status,
		ObservedGeneration: config.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	found := false
	for i, cond := range config.Status.Conditions {
		if cond.Type == ConditionTypeReady {
			config.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		config.Status.Conditions = append(config.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, config)
}

// SetupWithManager sets up the controller with the Manager
func (r *WolConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch for changes to WolConfig
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&wolv1beta1.WolConfig{}).
		Named("wol-wolconfig")

	// Watch VirtualMachines to trigger reconciliation when VMs change
	builder = builder.Watches(
		&kubevirtv1.VirtualMachine{},
		handler.EnqueueRequestsFromMapFunc(r.mapVMToConfig),
	)

	return builder.Complete(r)
}

// mapVMToConfig maps VirtualMachine changes to WolConfig reconciliation requests
func (r *WolConfigReconciler) mapVMToConfig(ctx context.Context, obj client.Object) []ctrl.Request {
	// List all WolConfigs (should typically be just one)
	configList := &wolv1beta1.WolConfigList{}
	if err := r.List(ctx, configList); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list WolConfigs")
		return []ctrl.Request{}
	}

	// Trigger reconciliation for all configs
	requests := make([]ctrl.Request, 0, len(configList.Items))
	for _, config := range configList.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name: config.Name,
			},
		})
	}

	return requests
}

// refreshAllConfigs refreshes VM mappings from ALL WolConfigs and merges them
// This allows multiple configs to work in OR mode
func (r *WolConfigReconciler) refreshAllConfigs(ctx context.Context) (int, error) {
	// List all WolConfigs
	configList := &wolv1beta1.WolConfigList{}
	if err := r.List(ctx, configList); err != nil {
		return 0, fmt.Errorf("failed to list WolConfigs: %w", err)
	}

	// Create a synthetic merged config that combines all WolConfigs
	// This implements OR logic: union of all configs
	mergedConfig := &wolv1beta1.WolConfig{
		Spec: wolv1beta1.WolConfigSpec{
			DiscoveryMode: wolv1beta1.DiscoveryModeAll,
		},
	}

	// Collect all namespace selectors from all configs
	allNamespaces := make(map[string]bool)
	allExplicitMappings := []wolv1beta1.MACVMMapping{}

	for _, config := range configList.Items {
		switch config.Spec.DiscoveryMode {
		case wolv1beta1.DiscoveryModeAll:
			// Add all namespaces from this config
			for _, ns := range config.Spec.NamespaceSelectors {
				allNamespaces[ns] = true
			}
		case wolv1beta1.DiscoveryModeExplicit:
			// Add explicit mappings
			allExplicitMappings = append(allExplicitMappings, config.Spec.ExplicitMappings...)
		case wolv1beta1.DiscoveryModeLabelSelector:
			// For label selectors, we need to discover in their namespaces
			tempMapper := wol.NewMACMapper(r.Client, ctrl.Log.WithName("mapper"))
			tempMapper.UpdateConfig(&config)
			if err := tempMapper.RefreshMapping(ctx); err != nil {
				ctrl.Log.Error(err, "Failed to refresh label selector config", "config", config.Name)
			}
		}
	}

	// Convert namespace map to slice
	if len(allNamespaces) > 0 {
		mergedConfig.Spec.NamespaceSelectors = make([]string, 0, len(allNamespaces))
		for ns := range allNamespaces {
			mergedConfig.Spec.NamespaceSelectors = append(mergedConfig.Spec.NamespaceSelectors, ns)
		}
	}

	// If we have explicit mappings, use them
	if len(allExplicitMappings) > 0 {
		mergedConfig.Spec.DiscoveryMode = wolv1beta1.DiscoveryModeExplicit
		mergedConfig.Spec.ExplicitMappings = allExplicitMappings
	}

	// Update the global mapper with merged config
	r.Mapper.UpdateConfig(mergedConfig)
	if err := r.Mapper.RefreshMapping(ctx); err != nil {
		return 0, fmt.Errorf("failed to refresh merged mapping: %w", err)
	}

	return r.Mapper.GetMappingCount(), nil
}
