# 1. Ottieni l'IP del nodo dove gira il pod
NODE_IP=$(oc get node <NODE_NAME> -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
echo "Node IP: $NODE_IP"

# 2. Invia WOL packet direttamente a quel nodo
./hack/test-wol.sh 02:f1:ef:00:00:13 $NODE_IP

# 3. Oppure usa wakeonlan
# wakeonlan -i $NODE_IP -p 9 02:f1:ef:00:00:0b