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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

var _ = Describe("StartupReconciler", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx              context.Context
		startupRecon     *StartupReconciler
		wolConfig        *wolv1beta1.WolConfig
		daemonSet        *appsv1.DaemonSet
		expectedImage    = "quay.io/test/agent:v2.0.0"
		outdatedImage    = "quay.io/test/agent:v1.0.0"
		wolConfigName    = "test-wol-config"
		daemonSetName    = "wol-agent-test-wol-config"
		testNamespace    = DefaultOperatorNamespace
		cleanupResources []func()
	)

	BeforeEach(func() {
		ctx = context.Background()
		cleanupResources = []func(){}

		// Create WolConfig
		wolConfig = &wolv1beta1.WolConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: wolConfigName,
			},
			Spec: wolv1beta1.WolConfigSpec{
				DiscoveryMode:      wolv1beta1.DiscoveryModeAll,
				NamespaceSelectors: []string{"default"},
				WOLPorts:           []int{9},
			},
		}
		Expect(k8sClient.Create(ctx, wolConfig)).To(Succeed())
		cleanupResources = append(cleanupResources, func() {
			_ = k8sClient.Delete(ctx, wolConfig)
		})

		// Create DaemonSet with outdated image
		daemonSet = &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName,
				Namespace: testNamespace,
				Labels: map[string]string{
					"app":                      "wol-agent",
					"wol.pillon.org/wolconfig": wolConfigName,
				},
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "wol-agent",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "wol-agent",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "agent",
								Image: outdatedImage,
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, daemonSet)).To(Succeed())
		cleanupResources = append(cleanupResources, func() {
			_ = k8sClient.Delete(ctx, daemonSet)
		})

		// Create startup reconciler
		startupRecon = &StartupReconciler{
			Client:     k8sClient,
			AgentImage: expectedImage,
			Log:        ctrl.Log.WithName("test-startup"),
		}
	})

	AfterEach(func() {
		// Clean up in reverse order
		for i := len(cleanupResources) - 1; i >= 0; i-- {
			cleanupResources[i]()
		}
	})

	Context("When checking DaemonSets at startup", func() {
		It("Should trigger reconciliation when image is outdated", func() {
			// Run the check
			err := startupRecon.checkAndUpdateDaemonSets(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify that WolConfig annotation was updated
			updatedConfig := &wolv1beta1.WolConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: wolConfigName}, updatedConfig)
				if err != nil {
					return false
				}
				_, ok := updatedConfig.Annotations["wol.pillon.org/last-image-check"]
				return ok
			}, timeout, interval).Should(BeTrue())
		})

		It("Should skip DaemonSets with explicit image override", func() {
			// Update WolConfig with explicit image
			Eventually(func() error {
				updatedConfig := &wolv1beta1.WolConfig{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: wolConfigName}, updatedConfig); err != nil {
					return err
				}
				updatedConfig.Spec.Agent.Image = outdatedImage // Explicit override
				return k8sClient.Update(ctx, updatedConfig)
			}, timeout, interval).Should(Succeed())

			// Get initial annotation state
			initialConfig := &wolv1beta1.WolConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wolConfigName}, initialConfig)).To(Succeed())
			initialAnnotation := ""
			if initialConfig.Annotations != nil {
				initialAnnotation = initialConfig.Annotations["wol.pillon.org/last-image-check"]
			}

			// Run the check
			err := startupRecon.checkAndUpdateDaemonSets(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify that annotation was NOT updated (since there's an explicit override)
			finalConfig := &wolv1beta1.WolConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wolConfigName}, finalConfig)).To(Succeed())
			finalAnnotation := ""
			if finalConfig.Annotations != nil {
				finalAnnotation = finalConfig.Annotations["wol.pillon.org/last-image-check"]
			}
			Expect(finalAnnotation).To(Equal(initialAnnotation))
		})

		It("Should skip DaemonSets when image matches", func() {
			// Update DaemonSet with expected image
			Eventually(func() error {
				ds := &appsv1.DaemonSet{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      daemonSetName,
					Namespace: testNamespace,
				}, ds); err != nil {
					return err
				}
				ds.Spec.Template.Spec.Containers[0].Image = expectedImage
				return k8sClient.Update(ctx, ds)
			}, timeout, interval).Should(Succeed())

			// Run the check
			err := startupRecon.checkAndUpdateDaemonSets(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify that annotation was NOT added
			finalConfig := &wolv1beta1.WolConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wolConfigName}, finalConfig)).To(Succeed())
			_, hasAnnotation := finalConfig.Annotations["wol.pillon.org/last-image-check"]
			Expect(hasAnnotation).To(BeFalse())
		})
	})
})
