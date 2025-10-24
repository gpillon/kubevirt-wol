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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// WOLPacketsTotal counts the number of Wake-on-LAN packets received
	WOLPacketsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wol_packets_total",
			Help: "Number of Wake-on-LAN packets received",
		},
	)

	// VMStartedTotal counts the number of VMs started via WOL
	VMStartedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wol_vm_started_total",
			Help: "Number of VMs started via WOL",
		},
	)

	// ErrorsTotal counts the number of errors during WOL handling
	ErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "wol_errors_total",
			Help: "Number of errors during WOL handling",
		},
	)

	// ManagedVMs is a gauge for the number of currently managed VMs
	ManagedVMs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "wol_managed_vms",
			Help: "Number of VMs currently being monitored for WOL",
		},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		WOLPacketsTotal,
		VMStartedTotal,
		ErrorsTotal,
		ManagedVMs,
	)
}
