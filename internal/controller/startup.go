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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

// StartupReconciler is a runnable that checks and updates DaemonSets at startup
type StartupReconciler struct {
	client.Client
	AgentImage        string
	OperatorNamespace string
	Log               logr.Logger
}

// Start implements the Runnable interface
func (s *StartupReconciler) Start(ctx context.Context) error {
	s.Log.Info("Starting DaemonSet image drift detection at startup")

	// Give the manager some time to initialize before checking
	time.Sleep(2 * time.Second)

	if err := s.checkAndUpdateDaemonSets(ctx); err != nil {
		s.Log.Error(err, "Failed to check and update DaemonSets at startup")
		// Don't fail the manager startup, just log the error
		return nil
	}

	s.Log.Info("Completed DaemonSet image drift detection")
	return nil
}

// checkAndUpdateDaemonSets lists all managed DaemonSets and updates them if image doesn't match
func (s *StartupReconciler) checkAndUpdateDaemonSets(ctx context.Context) error {
	// Determine namespace
	namespace := s.OperatorNamespace
	if namespace == "" {
		namespace = DefaultOperatorNamespace
	}

	// List all DaemonSets in the agent namespace
	dsList := &appsv1.DaemonSetList{}
	if err := s.List(ctx, dsList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list DaemonSets: %w", err)
	}

	s.Log.Info("Checking existing DaemonSets for image drift",
		"count", len(dsList.Items),
		"expectedImage", s.AgentImage)

	updatedCount := 0
	for i := range dsList.Items {
		ds := &dsList.Items[i]

		// Check if this is a wol-agent DaemonSet by checking labels
		if ds.Labels["app"] != "wol-agent" {
			continue
		}

		// Get the WolConfig name from labels
		wolConfigName, ok := ds.Labels["wol.pillon.org/wolconfig"]
		if !ok {
			s.Log.Info("DaemonSet missing WolConfig label, skipping",
				"daemonset", ds.Name)
			continue
		}

		// Get current image from DaemonSet
		if len(ds.Spec.Template.Spec.Containers) == 0 {
			s.Log.Info("DaemonSet has no containers, skipping",
				"daemonset", ds.Name)
			continue
		}

		currentImage := ds.Spec.Template.Spec.Containers[0].Image
		s.Log.Info("Checking DaemonSet image",
			"daemonset", ds.Name,
			"wolconfig", wolConfigName,
			"currentImage", currentImage,
			"expectedImage", s.AgentImage)

		// Check if image needs update
		// Skip if AgentImage is not set (using default or user override)
		if s.AgentImage == "" {
			s.Log.Info("AGENT_IMAGE not set, skipping image check for DaemonSet",
				"daemonset", ds.Name)
			continue
		}

		// Get the WolConfig to check if it has an explicit image override
		wolConfig := &wolv1beta1.WolConfig{}
		if err := s.Get(ctx, types.NamespacedName{Name: wolConfigName}, wolConfig); err != nil {
			s.Log.Error(err, "Failed to get WolConfig for DaemonSet",
				"daemonset", ds.Name,
				"wolconfig", wolConfigName)
			continue
		}

		// If WolConfig has an explicit image override, don't update
		if wolConfig.Spec.Agent.Image != "" {
			s.Log.Info("WolConfig has explicit image override, skipping update",
				"daemonset", ds.Name,
				"wolconfig", wolConfigName,
				"overrideImage", wolConfig.Spec.Agent.Image)
			continue
		}

		// Check if image matches
		if currentImage == s.AgentImage {
			s.Log.Info("DaemonSet image is up to date",
				"daemonset", ds.Name,
				"image", currentImage)
			continue
		}

		// Image drift detected - trigger reconciliation of the WolConfig
		s.Log.Info("Image drift detected, triggering WolConfig reconciliation",
			"daemonset", ds.Name,
			"wolconfig", wolConfigName,
			"currentImage", currentImage,
			"newImage", s.AgentImage)

		// Update the WolConfig annotation to trigger reconciliation
		if wolConfig.Annotations == nil {
			wolConfig.Annotations = make(map[string]string)
		}
		wolConfig.Annotations["wol.pillon.org/last-image-check"] = time.Now().Format(time.RFC3339)

		if err := s.Update(ctx, wolConfig); err != nil {
			s.Log.Error(err, "Failed to trigger WolConfig reconciliation",
				"wolconfig", wolConfigName)
			continue
		}

		updatedCount++
		s.Log.Info("Successfully triggered reconciliation for outdated DaemonSet",
			"daemonset", ds.Name,
			"wolconfig", wolConfigName)
	}

	if updatedCount > 0 {
		s.Log.Info("Image drift detection complete",
			"updated", updatedCount)
	} else {
		s.Log.Info("No image drift detected, all DaemonSets are up to date")
	}

	return nil
}
