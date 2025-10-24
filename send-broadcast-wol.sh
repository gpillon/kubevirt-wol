#!/bin/bash
# Test WOL broadcast packet sender
# Usage: ./send-broadcast-wol.sh <MAC_ADDRESS> [BROADCAST_IP] [PORT]

MAC="${1:-02:f1:ef:00:00:0b}"  # Default to a test MAC
BROADCAST_IP="${2:-255.255.255.255}"  # Broadcast to all networks
PORT="${3:-9}"

echo "Sending WOL broadcast packet:"
echo "  MAC Address: $MAC"
echo "  Broadcast IP: $BROADCAST_IP"
echo "  Port: $PORT"
echo ""

# Remove colons from MAC
MAC_CLEAN=$(echo "$MAC" | tr -d ':')

# Build the magic packet:
# - 6 bytes of 0xFF
# - 16 repetitions of the MAC address
MAGIC_PACKET="FFFFFFFFFFFF"
for i in {1..16}; do
    MAGIC_PACKET="${MAGIC_PACKET}${MAC_CLEAN}"
done

# Convert hex string to binary and send via UDP broadcast
echo -n "$MAGIC_PACKET" | xxd -r -p | socat - UDP-DATAGRAM:${BROADCAST_IP}:${PORT},broadcast

if [ $? -eq 0 ]; then
    echo "✓ WOL packet sent successfully"
else
    echo "✗ Failed to send WOL packet"
    exit 1
fi

