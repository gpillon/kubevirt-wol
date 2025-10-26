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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
	"github.com/gpillon/kubevirt-wol/internal/wol"
)

var _ = Describe("WolConfig Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a WolConfig", func() {
		var (
			ctx               context.Context
			namespace         *corev1.Namespace
			operatorNamespace *corev1.Namespace
			reconciler        *WolConfigReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()

			// Create test namespace
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, namespace)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			}

			// Create operator namespace for DaemonSet
			operatorNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultOperatorNamespace,
				},
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: DefaultOperatorNamespace}, operatorNamespace)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, operatorNamespace)).To(Succeed())
			}

			// Initialize reconciler with required components
			mapper := wol.NewMACMapper(k8sClient, ctrl.Log.WithName("mapper"))
			vmStarter := wol.NewVMStarter(k8sClient, ctrl.Log.WithName("vmstarter"))

			reconciler = &WolConfigReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Mapper:    mapper,
				VMStarter: vmStarter,
			}
		})

		AfterEach(func() {
			// Cleanup WolConfigs
			configList := &wolv1beta1.WolConfigList{}
			Expect(k8sClient.List(ctx, configList)).To(Succeed())
			for _, config := range configList.Items {
				Expect(k8sClient.Delete(ctx, &config)).To(Succeed())
			}
		})

		It("should successfully reconcile a basic WolConfig with All discovery mode", func() {
			config := &wolv1beta1.WolConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config-all",
				},
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode:      wolv1beta1.DiscoveryModeAll,
					NamespaceSelectors: []string{"default"},
					WOLPorts:           []int{9},
					CacheTTL:           300,
				},
			}

			By("Creating the WolConfig")
			Expect(k8sClient.Create(ctx, config)).To(Succeed())

			By("Reconciling the WolConfig")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: config.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: config.Name}, config)
				return err == nil && len(config.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			By("Verifying the Ready condition")
			Expect(config.Status.Conditions).NotTo(BeEmpty())
			readyCondition := config.Status.Conditions[0]
			Expect(readyCondition.Type).To(Equal(ConditionTypeReady))
		})

		It("should successfully reconcile a WolConfig with Explicit discovery mode", func() {
			config := &wolv1beta1.WolConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config-explicit",
				},
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode: wolv1beta1.DiscoveryModeExplicit,
					ExplicitMappings: []wolv1beta1.MACVMMapping{
						{
							MACAddress: "52:54:00:12:34:56",
							VMName:     "test-vm",
							Namespace:  "default",
						},
					},
					WOLPorts: []int{9},
					CacheTTL: 300,
				},
			}

			By("Creating the WolConfig")
			Expect(k8sClient.Create(ctx, config)).To(Succeed())

			By("Reconciling the WolConfig")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: config.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: config.Name}, config)
				return err == nil && len(config.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("should fail validation for invalid WOL port", func() {
			config := &wolv1beta1.WolConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config-invalid-port",
				},
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode:      wolv1beta1.DiscoveryModeAll,
					NamespaceSelectors: []string{"default"},
					WOLPorts:           []int{99999}, // Invalid port
					CacheTTL:           300,
				},
			}

			By("Creating the WolConfig")
			Expect(k8sClient.Create(ctx, config)).To(Succeed())

			By("Reconciling the WolConfig")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: config.Name},
			})
			Expect(err).To(HaveOccurred())
		})

		It("should handle WolConfig deletion", func() {
			config := &wolv1beta1.WolConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config-delete",
				},
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode:      wolv1beta1.DiscoveryModeAll,
					NamespaceSelectors: []string{"default"},
					WOLPorts:           []int{9},
					CacheTTL:           300,
				},
			}

			By("Creating the WolConfig")
			Expect(k8sClient.Create(ctx, config)).To(Succeed())

			By("Reconciling the WolConfig")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: config.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the WolConfig")
			Expect(k8sClient.Delete(ctx, config)).To(Succeed())

			By("Reconciling after deletion")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: config.Name},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating WolConfig", func() {
		var reconciler *WolConfigReconciler

		BeforeEach(func() {
			reconciler = &WolConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should set default values for empty fields", func() {
			config := &wolv1beta1.WolConfig{
				Spec: wolv1beta1.WolConfigSpec{},
			}

			err := reconciler.validateConfig(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.Spec.DiscoveryMode).To(Equal(wolv1beta1.DiscoveryModeAll))
			Expect(config.Spec.WOLPorts).To(Equal([]int{9}))
			Expect(int32(config.Spec.CacheTTL)).To(Equal(int32(300)))
		})

		It("should reject invalid port numbers", func() {
			config := &wolv1beta1.WolConfig{
				Spec: wolv1beta1.WolConfigSpec{
					WOLPorts: []int{0},
				},
			}

			err := reconciler.validateConfig(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid WOL port"))
		})

		It("should reject LabelSelector mode without VMSelector", func() {
			config := &wolv1beta1.WolConfig{
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode: wolv1beta1.DiscoveryModeLabelSelector,
				},
			}

			err := reconciler.validateConfig(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("VMSelector is required"))
		})

		It("should reject Explicit mode without ExplicitMappings", func() {
			config := &wolv1beta1.WolConfig{
				Spec: wolv1beta1.WolConfigSpec{
					DiscoveryMode: wolv1beta1.DiscoveryModeExplicit,
				},
			}

			err := reconciler.validateConfig(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ExplicitMappings is required"))
		})
	})
})
