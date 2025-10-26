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

package wol

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VMStarter handles starting VirtualMachines
type VMStarter struct {
	client client.Client
	log    logr.Logger
}

// NewVMStarter creates a new VM starter
func NewVMStarter(k8sClient client.Client, log logr.Logger) *VMStarter {
	return &VMStarter{
		client: k8sClient,
		log:    log,
	}
}

// StartVM starts a VirtualMachine using KubeVirt subresource API
func (s *VMStarter) StartVM(ctx context.Context, namespace, name string) error {
	vm := &kubevirtv1.VirtualMachine{}
	key := client.ObjectKey{Namespace: namespace, Name: name}

	// Get the VM to check current state
	if err := s.client.Get(ctx, key, vm); err != nil {
		ErrorsTotal.Inc()
		return fmt.Errorf("failed to get VM %s/%s: %w", namespace, name, err)
	}

	// Check if VM is already running by looking at actual status
	if vm.Spec.RunStrategy != nil {
		// VM uses RunStrategy (modern approach)

		// Check if VM is actually running (not just configured to run)
		isRunning := vm.Status.Ready ||
			(vm.Status.PrintableStatus != "" &&
				(vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusRunning ||
					vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusStarting))

		if isRunning {
			s.log.Info("VM is already running", "vm", name, "namespace", namespace, "runStrategy", *vm.Spec.RunStrategy)
			return nil
		}

		// For strategies that need temporary change to start the VM
		needsRestore := *vm.Spec.RunStrategy == kubevirtv1.RunStrategyOnce ||
			*vm.Spec.RunStrategy == kubevirtv1.RunStrategyRerunOnFailure ||
			*vm.Spec.RunStrategy == kubevirtv1.RunStrategyManual

		if needsRestore {
			// Save original strategy
			originalStrategy := *vm.Spec.RunStrategy

			// Set to Always to start the VM
			patch := client.MergeFrom(vm.DeepCopy())
			runStrategy := kubevirtv1.RunStrategyAlways
			vm.Spec.RunStrategy = &runStrategy

			if err := s.client.Patch(ctx, vm, patch); err != nil {
				ErrorsTotal.Inc()
				return fmt.Errorf("failed to start VM %s/%s: %w", namespace, name, err)
			}

			s.log.Info("Temporarily changed RunStrategy to start VM", "vm", name, "namespace", namespace, "originalStrategy", originalStrategy)
			VMStartedTotal.Inc()

			// Start goroutine to restore original strategy after VM is running
			go s.restoreStrategyWhenRunning(context.Background(), namespace, name, originalStrategy)

			return nil
		}

		// For other strategies (Always, Halted), just ensure it's set correctly
		if *vm.Spec.RunStrategy != kubevirtv1.RunStrategyAlways {
			patch := client.MergeFrom(vm.DeepCopy())
			runStrategy := kubevirtv1.RunStrategyAlways
			vm.Spec.RunStrategy = &runStrategy

			if err := s.client.Patch(ctx, vm, patch); err != nil {
				ErrorsTotal.Inc()
				return fmt.Errorf("failed to start VM %s/%s: %w", namespace, name, err)
			}

			s.log.Info("Changed RunStrategy to start VM", "vm", name, "namespace", namespace)
			VMStartedTotal.Inc()
		}

		return nil
	}

	// Fallback to deprecated Running field if RunStrategy not set
	if vm.Spec.Running != nil && *vm.Spec.Running {
		s.log.Info("VM is already running", "vm", name, "namespace", namespace)
		return nil
	}

	// Start the VM by setting Running to true (deprecated but still supported)
	patch := client.MergeFrom(vm.DeepCopy())
	running := true
	vm.Spec.Running = &running

	if err := s.client.Patch(ctx, vm, patch); err != nil {
		ErrorsTotal.Inc()
		return fmt.Errorf("failed to start VM %s/%s: %w", namespace, name, err)
	}

	s.log.Info("Successfully started VM via Running field", "vm", name, "namespace", namespace)
	VMStartedTotal.Inc()
	return nil
}

// restoreStrategyWhenRunning waits for VM to be running, then restores original RunStrategy
func (s *VMStarter) restoreStrategyWhenRunning(ctx context.Context, namespace, name string, originalStrategy kubevirtv1.VirtualMachineRunStrategy) {
	maxAttempts := 60 // 5 minutes max wait (5 seconds * 60)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		time.Sleep(5 * time.Second)

		vm := &kubevirtv1.VirtualMachine{}
		key := client.ObjectKey{Namespace: namespace, Name: name}

		if err := s.client.Get(ctx, key, vm); err != nil {
			s.log.Error(err, "Failed to get VM for strategy restore", "vm", name, "namespace", namespace)
			continue
		}

		// Check if VM is running
		isRunning := vm.Status.Ready ||
			(vm.Status.PrintableStatus != "" &&
				vm.Status.PrintableStatus == kubevirtv1.VirtualMachineStatusRunning)

		if isRunning {
			// VM is running, restore original strategy
			patch := client.MergeFrom(vm.DeepCopy())
			vm.Spec.RunStrategy = &originalStrategy

			if err := s.client.Patch(ctx, vm, patch); err != nil {
				s.log.Error(err, "Failed to restore original RunStrategy", "vm", name, "namespace", namespace, "originalStrategy", originalStrategy)
				return
			}

			s.log.Info("Restored original RunStrategy after VM started", "vm", name, "namespace", namespace, "strategy", originalStrategy)
			return
		}
	}

	s.log.Info("Timeout waiting for VM to start, keeping Always strategy", "vm", name, "namespace", namespace)
}

// IsVMRunning checks if a VM is currently running
func (s *VMStarter) IsVMRunning(ctx context.Context, namespace, name string) (bool, error) {
	vm := &kubevirtv1.VirtualMachine{}
	key := client.ObjectKey{Namespace: namespace, Name: name}

	if err := s.client.Get(ctx, key, vm); err != nil {
		return false, fmt.Errorf("failed to get VM %s/%s: %w", namespace, name, err)
	}

	if vm.Spec.Running != nil && *vm.Spec.Running {
		return true, nil
	}

	// Also check the status to see if VM is actually running
	if vm.Status.Ready {
		return true, nil
	}

	return false, nil
}
