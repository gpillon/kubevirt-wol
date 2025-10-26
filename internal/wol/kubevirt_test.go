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
)

func TestNewVMStarter(t *testing.T) {
	logger := logr.Discard()
	starter := NewVMStarter(nil, logger)
	if starter == nil {
		t.Fatal("NewVMStarter returned nil")
	}

	if starter.client != nil {
		t.Error("Expected nil client")
	}

	// Verify logger is stored (even if it's a discard logger)
	if starter.log.GetSink() != logger.GetSink() {
		t.Error("Expected logger to be stored")
	}
}
