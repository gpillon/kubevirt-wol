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
)

// ConfigReconciler reconciles a WOLConfig object
type ConfigReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Mapper    *wol.MACMapper
	Listener  *wol.Listener
	VMStarter *wol.VMStarter
}

// +kubebuilder:rbac:groups=wol.pillon.org,resources=configs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=wol.pillon.org,resources=configs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=wol.pillon.org,resources=configs/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=subresources.kubevirt.io,resources=virtualmachines/start,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile handles WOLConfig reconciliation
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the WOLConfig instance
	config := &wolv1beta1.Config{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if errors.IsNotFound(err) {
			// Config deleted, nothing to do
			log.Info("WOLConfig deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get WOLConfig")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling WOLConfig",
		"name", config.Name,
		"discoveryMode", config.Spec.DiscoveryMode,
		"wolPort", config.Spec.WOLPort)

	// Validate configuration
	if err := r.validateConfig(config); err != nil {
		log.Error(err, "Invalid configuration")
		r.updateStatus(ctx, config, false, ReasonInvalidConfig, err.Error())
		return ctrl.Result{}, err
	}

	// Refresh global mapping from ALL WOLConfigs (not just this one)
	// This ensures multiple configs work in OR mode, not AND
	managedVMs, err := r.refreshAllConfigs(ctx)
	if err != nil {
		log.Error(err, "Failed to refresh VM mapping from all configs")
		r.updateStatus(ctx, config, false, ReasonInvalidConfig, fmt.Sprintf("Failed to refresh mapping: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status for this specific config
	now := metav1.Now()
	config.Status.ManagedVMs = managedVMs
	config.Status.LastSync = &now

	if err := r.updateStatus(ctx, config, true, ReasonMappingUpdated, "VM mapping refreshed successfully"); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled WOLConfig",
		"managedVMs", config.Status.ManagedVMs,
		"lastSync", config.Status.LastSync)

	// Requeue to refresh mapping periodically
	requeueAfter := time.Duration(config.Spec.CacheTTL) * time.Second
	if requeueAfter == 0 {
		requeueAfter = 5 * time.Minute
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// validateConfig validates the WOLConfig specification
func (r *ConfigReconciler) validateConfig(config *wolv1beta1.Config) error {
	// Validate discovery mode
	if config.Spec.DiscoveryMode == "" {
		config.Spec.DiscoveryMode = wolv1beta1.DiscoveryModeAll
	}

	// Validate WOL port
	if config.Spec.WOLPort == 0 {
		config.Spec.WOLPort = 9
	}
	if config.Spec.WOLPort < 1 || config.Spec.WOLPort > 65535 {
		return fmt.Errorf("invalid WOL port: %d (must be 1-65535)", config.Spec.WOLPort)
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

// updateStatus updates the WOLConfig status
func (r *ConfigReconciler) updateStatus(ctx context.Context, config *wolv1beta1.Config, ready bool, reason, message string) error {
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
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch for changes to WOLConfig
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&wolv1beta1.Config{})

	// Watch VirtualMachines to trigger reconciliation when VMs change
	builder = builder.Watches(
		&kubevirtv1.VirtualMachine{},
		handler.EnqueueRequestsFromMapFunc(r.mapVMToConfig),
	)

	return builder.Complete(r)
}

// mapVMToConfig maps VirtualMachine changes to WOLConfig reconciliation requests
func (r *ConfigReconciler) mapVMToConfig(ctx context.Context, obj client.Object) []ctrl.Request {
	// List all WOLConfigs (should typically be just one)
	configList := &wolv1beta1.ConfigList{}
	if err := r.List(ctx, configList); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list WOLConfigs")
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

// refreshAllConfigs refreshes VM mappings from ALL WOLConfigs and merges them
// This allows multiple configs to work in OR mode
func (r *ConfigReconciler) refreshAllConfigs(ctx context.Context) (int, error) {
	// List all WOLConfigs
	configList := &wolv1beta1.ConfigList{}
	if err := r.List(ctx, configList); err != nil {
		return 0, fmt.Errorf("failed to list WOLConfigs: %w", err)
	}

	// Create a synthetic merged config that combines all WOLConfigs
	// This implements OR logic: union of all configs
	mergedConfig := &wolv1beta1.Config{
		Spec: wolv1beta1.ConfigSpec{
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
