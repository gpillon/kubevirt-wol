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
	"testing"

	"github.com/go-logr/logr"
	wolv1 "github.com/gpillon/kubevirt-wol/api/wol/v1"
)

func TestNewAggregator(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())
	vmStarter := NewVMStarter(nil, logr.Discard())

	agg := NewAggregator(mapper, vmStarter, logr.Discard())
	if agg == nil {
		t.Fatal("NewAggregator returned nil")
	}

	if agg.mapper == nil {
		t.Error("Aggregator mapper is nil")
	}

	if agg.vmStarter == nil {
		t.Error("Aggregator vmStarter is nil")
	}

	if agg.dedupeMap == nil {
		t.Error("Aggregator dedupeMap is nil")
	}
}

func TestAggregator_ReportWOLEvent_UnknownMAC(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())
	vmStarter := NewVMStarter(nil, logr.Discard())
	agg := NewAggregator(mapper, vmStarter, logr.Discard())

	req := &wolv1.WOLEvent{
		MacAddress: "52:54:00:12:34:56",
		NodeName:   "test-node",
		SourceIp:   "192.168.1.1",
	}

	resp, err := agg.ReportWOLEvent(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.Status == wolv1.ResponseStatus_ACCEPTED {
		t.Error("Expected event to not be accepted for unknown MAC")
	}

	if resp.Status != wolv1.ResponseStatus_VM_NOT_FOUND {
		t.Errorf("Expected VM_NOT_FOUND status, got %v", resp.Status)
	}
}

func TestAggregator_HealthCheck(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())
	vmStarter := NewVMStarter(nil, logr.Discard())
	agg := NewAggregator(mapper, vmStarter, logr.Discard())

	req := &wolv1.HealthCheckRequest{}
	resp, err := agg.HealthCheck(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.Status != wolv1.HealthCheckResponse_SERVING {
		t.Errorf("Expected SERVING status, got %v", resp.Status)
	}
}

func TestAggregator_Deduplication(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())
	vmStarter := NewVMStarter(nil, logr.Discard())
	agg := NewAggregator(mapper, vmStarter, logr.Discard())

	// First event should not be duplicate
	req1 := &wolv1.WOLEvent{
		MacAddress: "52:54:00:12:34:56",
		NodeName:   "test-node",
		SourceIp:   "192.168.1.1",
	}

	resp1, err := agg.ReportWOLEvent(context.Background(), req1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp1.WasDuplicate {
		t.Error("First event should not be marked as duplicate")
	}

	// Second identical event should be duplicate
	resp2, err := agg.ReportWOLEvent(context.Background(), req1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !resp2.WasDuplicate {
		t.Error("Second identical event should be marked as duplicate")
	}

	if resp2.Status != wolv1.ResponseStatus_DUPLICATE {
		t.Errorf("Expected DUPLICATE status, got %v", resp2.Status)
	}
}
