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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

// updateAgentStatus updates the WolConfig status with DaemonSet information
func (r *WolConfigReconciler) updateAgentStatus(ctx context.Context, wolConfig *wolv1beta1.WolConfig) error {
	daemonSetName := getDaemonSetName(wolConfig)

	namespace := r.OperatorNamespace
	if namespace == "" {
		namespace = DefaultOperatorNamespace
	}

	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      daemonSetName,
		Namespace: namespace,
	}, ds)

	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet not created yet, clear status
			wolConfig.Status.AgentStatus = nil
			return nil
		}
		return err
	}

	// Update status from DaemonSet
	wolConfig.Status.AgentStatus = &wolv1beta1.AgentStatus{
		DaemonSetName:          daemonSetName,
		DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
		NumberReady:            ds.Status.NumberReady,
		NumberAvailable:        ds.Status.NumberAvailable,
	}

	return nil
}
