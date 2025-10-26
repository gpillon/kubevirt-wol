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

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gpillon/kubevirt-wol/test/utils"
)

const namespace = "kubevirt-wol-system"

var _ = Describe("controller", Ordered, func() {
	BeforeAll(func() {
		By("installing prometheus operator")
		Expect(utils.InstallPrometheusOperator()).To(Succeed())

		By("installing the cert-manager")
		Expect(utils.InstallCertManager()).To(Succeed())

		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterAll(func() {
		By("uninstalling the Prometheus manager bundle")
		utils.UninstallPrometheusOperator()

		By("uninstalling the cert-manager bundle")
		utils.UninstallCertManager()

		By("removing manager namespace")
		cmd := exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	Context("Operator", func() {
		It("should run successfully", func() {
			var controllerPodName string
			var err error

			// projectimage stores the name of the image used in the example
			var projectimage = "example.com/kubevirt-wol:v0.0.1"

			By("building the manager(Operator) image")
			cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("loading the the manager(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(projectimage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("installing CRDs")
			cmd = exec.Command("make", "install")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("deploying the controller-manager")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func() error {
				// Get pod name

				cmd = exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) != 1 {
					return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
				}
				controllerPodName = podNames[0]
				ExpectWithOffset(2, controllerPodName).Should(ContainSubstring("controller-manager"))

				// Validate pod status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				status, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				if string(status) != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyControllerUp, time.Minute, time.Second).Should(Succeed())

			By("validating that ServiceMonitor is created and configured correctly")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "servicemonitor",
					"kubevirt-wol-controller-manager-metrics-monitor",
					"-n", namespace,
					"-o", "jsonpath={.spec.endpoints[0].port}")
				output, err := utils.Run(cmd)
				if err != nil {
					return fmt.Errorf("ServiceMonitor not found: %w", err)
				}
				if string(output) != "https" {
					return fmt.Errorf("ServiceMonitor port is %s, expected https", output)
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())

			By("creating a WolConfig to trigger agent deployment")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/wol_v1beta1_wolconfig-default.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that agent DaemonSet is created")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "daemonset",
					"wol-agent-default",
					"-n", namespace,
					"-o", "jsonpath={.status.numberReady}")
				output, err := utils.Run(cmd)
				if err != nil {
					return fmt.Errorf("DaemonSet not found: %w", err)
				}
				if string(output) == "0" || string(output) == "" {
					return fmt.Errorf("no agent pods ready yet")
				}
				return nil
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("validating that agents have hostNetwork enabled")
			cmd = exec.Command("kubectl", "get", "daemonset",
				"wol-agent-default",
				"-n", namespace,
				"-o", "jsonpath={.spec.template.spec.hostNetwork}")
			output, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			ExpectWithOffset(1, string(output)).To(Equal("true"))

			By("validating that manager does NOT have hostNetwork")
			cmd = exec.Command("kubectl", "get", "deployment",
				"kubevirt-wol-controller-manager",
				"-n", namespace,
				"-o", "jsonpath={.spec.template.spec.hostNetwork}")
			output, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			// Empty or false means hostNetwork is not set (good!)
			ExpectWithOffset(1, string(output)).To(BeEmpty())

			By("validating gRPC service is created")
			cmd = exec.Command("kubectl", "get", "service",
				"kubevirt-wol-kubevirt-wol-grpc",
				"-n", namespace,
				"-o", "jsonpath={.spec.ports[0].port}")
			output, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			ExpectWithOffset(1, string(output)).To(Equal("9090"))

			By("checking agent logs for successful gRPC connection")
			var agentPodName string
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "pods",
					"-l", "app=wol-agent",
					"-n", namespace,
					"-o", "go-template={{ range .items }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}")
				podOutput, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) == 0 {
					return fmt.Errorf("no agent pods found")
				}
				agentPodName = podNames[0]
				return nil
			}, time.Minute, 5*time.Second).Should(Succeed())

			By("verifying agent logs show gRPC connectivity")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "logs", agentPodName,
					"-n", namespace,
					"--tail", "50")
				logs, err := utils.Run(cmd)
				if err != nil {
					return fmt.Errorf("failed to get logs: %w", err)
				}
				logsStr := string(logs)
				// Agent should log about starting UDP listener and gRPC client
				if !utils.ContainsString(logsStr, "Starting UDP listener") &&
					!utils.ContainsString(logsStr, "Listening for WOL packets") {
					return fmt.Errorf("agent not listening for WOL packets yet")
				}
				return nil
			}, time.Minute, 5*time.Second).Should(Succeed())

			By("validating WolConfig status is updated")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "wolconfig",
					"default",
					"-o", "jsonpath={.status.agentDaemonSet}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if string(output) != "wol-agent-default" {
					return fmt.Errorf("status not updated, got: %s", output)
				}
				return nil
			}, time.Minute, 5*time.Second).Should(Succeed())

			By("checking controller logs for reconciliation")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "logs", controllerPodName,
					"-n", namespace,
					"--tail", "100")
				logs, err := utils.Run(cmd)
				if err != nil {
					return fmt.Errorf("failed to get controller logs: %w", err)
				}
				logsStr := string(logs)
				if !utils.ContainsString(logsStr, "Reconciling WolConfig") &&
					!utils.ContainsString(logsStr, "Creating DaemonSet") {
					return fmt.Errorf("controller not reconciling WolConfig")
				}
				return nil
			}, time.Minute, 5*time.Second).Should(Succeed())

			By("verifying metrics endpoint is accessible")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "run", "curl-test",
					"--image=curlimages/curl:latest",
					"--restart=Never",
					"--rm", "-i",
					"--", "curl", "-k",
					fmt.Sprintf("https://kubevirt-wol-controller-manager-metrics-service.%s.svc.cluster.local:8443/metrics", namespace))
				_, err := utils.Run(cmd)
				return err
			}, 2*time.Minute, 10*time.Second).Should(Succeed())

			By("cleaning up WolConfig")
			cmd = exec.Command("kubectl", "delete", "wolconfig", "default")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that agent DaemonSet is deleted via OwnerReference")
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "daemonset",
					"wol-agent-default",
					"-n", namespace)
				_, err := utils.Run(cmd)
				if err == nil {
					return fmt.Errorf("DaemonSet still exists")
				}
				return nil
			}, time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})
