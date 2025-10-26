#!/bin/bash

# Test script to verify WOL
# Usage: ./test-wol.sh <MAC_ADDRESS> [TARGET_IP]

MAC="${1:-52:54:00:12:34:56}"
TARGET="${2:-255.255.255.255}"  # broadcast by default
PORT="${3:-9}"

echo "=========================================="
echo "Testing Wake-on-LAN"
echo "=========================================="
echo "Target MAC: $MAC"
echo "Target IP:  $TARGET"
echo "Port:       $PORT (UDP)"
echo ""

# Check if wakeonlan is installed
if command -v wakeonlan &> /dev/null; then
    echo "Using wakeonlan command..."
    wakeonlan -i "$TARGET" -p $PORT "$MAC"
    echo "✓ WOL packet sent via wakeonlan"
else
    echo "wakeonlan not found, using Python..."
    
    # Create magic packet with Python
    python3 << EOF
import socket
import sys

mac = "${MAC}".replace(':', '')
if len(mac) != 12:
    print("Invalid MAC address")
    sys.exit(1)

# Create magic packet: 6 bytes FF + 16 repetitions of MAC
data = b'\xff' * 6 + bytes.fromhex(mac) * 16

# Send as UDP broadcast
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
sock.sendto(data, ('${TARGET}', $PORT))
sock.close()

print(f"✓ WOL magic packet sent to ${TARGET}:9")
print(f"  Packet size: {len(data)} bytes")
print(f"  Target MAC: ${MAC}")
EOF
fi

echo ""
echo "Checking operator logs (wait 2 seconds)..."
sleep 2

echo ""
echo "Recent logs:"
oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=10 --since=10s

echo ""
echo "=========================================="
echo "Troubleshooting Tips:"
echo "=========================================="
echo ""
echo "1. Check pod is on correct node:"
echo "   oc get pod -n kubevirt-wol-system -o wide"
echo ""
echo "2. Verify listener is running:"
echo "   oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep 'WOL listener started'"
echo ""
echo "3. Send to specific node IP:"
echo "   ./test-wol.sh $MAC <NODE_IP>"
echo ""
echo "4. Check from inside the pod:"
echo "   oc exec -n kubevirt-wol-system \$(oc get pod -n kubevirt-wol-system -l control-plane=controller-manager -o name) -- netstat -ulpn | grep :9"
echo ""
echo "5. Monitor logs in real-time:"
echo "   oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f"

