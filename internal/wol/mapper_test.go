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
	"testing"

	"github.com/go-logr/logr"
	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

func TestMACMapper_Lookup(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())

	// Test with empty mapping
	_, found := mapper.Lookup("52:54:00:12:34:56")
	if found {
		t.Error("Expected not found for empty mapping")
	}
}

func TestMACMapper_UpdateConfig(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())

	config := &wolv1beta1.WolConfig{
		Spec: wolv1beta1.WolConfigSpec{
			CacheTTL: 600,
		},
	}

	mapper.UpdateConfig(config)

	// Verify the cache TTL was updated
	if mapper.cacheTTL.Seconds() != 600 {
		t.Errorf("Expected cache TTL 600s, got %v", mapper.cacheTTL)
	}
}

func TestMACMapper_GetMappingCount(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())

	count := mapper.GetMappingCount()
	if count != 0 {
		t.Errorf("Expected mapping count 0, got %d", count)
	}
}

func TestMACMapper_NeedRefresh(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())

	// Initially should need refresh (no last sync)
	if !mapper.NeedRefresh() {
		t.Error("Expected NeedRefresh to return true initially")
	}
}

func TestMACMapper_GetLastSync(t *testing.T) {
	mapper := NewMACMapper(nil, logr.Discard())

	// Initially should return zero time
	lastSync := mapper.GetLastSync()
	if !lastSync.IsZero() {
		t.Errorf("Expected zero time for lastSync, got %v", lastSync)
	}
}
