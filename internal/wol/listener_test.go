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
)

func TestParseMagicPacket(t *testing.T) {
	tests := []struct {
		name      string
		packet    []byte
		wantMAC   string
		wantValid bool
	}{
		{
			name:      "valid magic packet",
			packet:    createValidMagicPacket([]byte{0x52, 0x54, 0x00, 0x12, 0x34, 0x56}),
			wantMAC:   "52:54:00:12:34:56",
			wantValid: true,
		},
		{
			name:      "packet too short",
			packet:    make([]byte, 50),
			wantMAC:   "",
			wantValid: false,
		},
		{
			name:      "invalid header",
			packet:    createInvalidHeaderPacket(),
			wantMAC:   "",
			wantValid: false,
		},
		{
			name:      "inconsistent MAC repetitions",
			packet:    createInvalidRepetitionPacket(),
			wantMAC:   "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac, valid := parseMagicPacket(tt.packet)
			if valid != tt.wantValid {
				t.Errorf("parseMagicPacket() valid = %v, want %v", valid, tt.wantValid)
			}
			if mac != tt.wantMAC {
				t.Errorf("parseMagicPacket() mac = %v, want %v", mac, tt.wantMAC)
			}
		})
	}
}

// createValidMagicPacket creates a valid WOL magic packet for testing
func createValidMagicPacket(mac []byte) []byte {
	if len(mac) != 6 {
		panic("MAC address must be 6 bytes")
	}

	// Create packet: 6 bytes 0xFF + 16 repetitions of MAC
	packet := make([]byte, 102)

	// Fill first 6 bytes with 0xFF
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	// Repeat MAC 16 times
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:6+(i+1)*6], mac)
	}

	return packet
}

// createInvalidHeaderPacket creates a packet with invalid header
func createInvalidHeaderPacket() []byte {
	packet := make([]byte, 102)
	// Don't fill with 0xFF - leave as zeros
	return packet
}

// createInvalidRepetitionPacket creates a packet with inconsistent MAC repetitions
func createInvalidRepetitionPacket() []byte {
	packet := make([]byte, 102)

	// Valid header
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}

	// First MAC
	mac1 := []byte{0x52, 0x54, 0x00, 0x12, 0x34, 0x56}
	copy(packet[6:12], mac1)

	// Second MAC (different)
	mac2 := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	copy(packet[12:18], mac2)

	// Fill rest with first MAC
	for i := 2; i < 16; i++ {
		copy(packet[6+i*6:6+(i+1)*6], mac1)
	}

	return packet
}

func BenchmarkParseMagicPacket(b *testing.B) {
	packet := createValidMagicPacket([]byte{0x52, 0x54, 0x00, 0x12, 0x34, 0x56})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseMagicPacket(packet)
	}
}
