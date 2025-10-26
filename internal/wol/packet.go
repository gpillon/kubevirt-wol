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

import "fmt"

const (
	// DefaultWOLPort is the standard Wake-on-LAN UDP port
	DefaultWOLPort = 9
	// MagicPacketSize is the minimum size of a WOL magic packet (6 + 6*16 = 102 bytes)
	MagicPacketSize = 6 + 16*6 // 6x0xFF + 16 repetitions of MAC

)

// parseMagicPacket validates and extracts the MAC address from a WOL magic packet
// A valid magic packet contains:
// - 6 bytes of 0xFF
// - 16 repetitions of the target MAC address (6 bytes each)
func parseMagicPacket(packet []byte) (string, bool) {
	// Check minimum size
	if len(packet) < MagicPacketSize {
		return "", false
	}

	// Check for 6 bytes of 0xFF at the start
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			return "", false
		}
	}

	// Extract the MAC address from the first repetition (bytes 6-11)
	macBytes := packet[6:12]

	// Verify that the MAC is repeated 16 times
	for i := 1; i < 16; i++ {
		offset := 6 + (i * 6)
		for j := 0; j < 6; j++ {
			if packet[offset+j] != macBytes[j] {
				return "", false
			}
		}
	}

	// Format MAC address as string (lowercase with colons)
	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		macBytes[0], macBytes[1], macBytes[2],
		macBytes[3], macBytes[4], macBytes[5])

	return mac, true
}
