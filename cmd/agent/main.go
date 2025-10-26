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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gpillon/kubevirt-wol/internal/wol"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var nodeName string
	var operatorAddr string
	var portsStr string

	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"),
		"Kubernetes node name (from downward API or env)")
	flag.StringVar(&operatorAddr, "operator-address",
		"kubevirt-wol-grpc.kubevirt-wol-system.svc.cluster.local:9090",
		"Operator gRPC address")
	flag.StringVar(&portsStr, "ports", "9", "UDP ports for WOL packets (comma-separated)")

	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)

	if nodeName == "" {
		setupLog.Error(nil, "node-name is required (use --node-name flag or NODE_NAME env var)")
		os.Exit(1)
	}

	// Parse ports (for now use first port only, TODO: multi-port support)
	ports, err := parsePorts(portsStr)
	if err != nil {
		setupLog.Error(err, "Failed to parse ports", "portsStr", portsStr)
		os.Exit(1)
	}

	port := ports[0] // Use first port for now

	setupLog.Info("Starting WOL Agent",
		"node", nodeName,
		"operator", operatorAddr,
		"port", port,
		"version", "v0.0.1")

	// Context con signal handling per graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Crea e avvia agent
	agent := wol.NewAgent(port, nodeName, operatorAddr, setupLog)

	if err := agent.Start(ctx); err != nil {
		setupLog.Error(err, "Agent failed to start")
		os.Exit(1)
	}

	setupLog.Info("Agent stopped gracefully")
}

func parsePorts(portsStr string) ([]int, error) {
	parts := strings.Split(portsStr, ",")
	ports := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %w", part, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of range (must be 1-65535)", port)
		}
		ports = append(ports, port)
	}

	if len(ports) == 0 {
		return []int{9}, nil // Default
	}

	return ports, nil
}
