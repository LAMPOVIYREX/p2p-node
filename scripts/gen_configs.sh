#!/usr/bin/env bash
# Usage: gen_configs.sh <topology>   # star | ring
set -euo pipefail

TOPO="${1:-star}"
N=21
BASE_PORT=9101
OUT="configs/$TOPO"

case "$TOPO" in
    star|ring) ;;
    *) echo "unknown topology: $TOPO (use star or ring)" >&2; exit 1 ;;
esac

mkdir -p "$OUT"
rm -f "$OUT"/node-*.yaml

for i in $(seq -w 1 $N); do
    port=$((BASE_PORT + 10#$i - 1))

    if [ "$i" = "01" ]; then
        peers="[]"
    elif [ "$TOPO" = "star" ]; then
        peers='["127.0.0.1:9101"]'
    else
        # ring: node-XX bootstrap'ится к node-(XX-1)
        prev=$(printf "%02d" $((10#$i - 1)))
        prevport=$((BASE_PORT + 10#$prev - 1))
        peers="[\"127.0.0.1:$prevport\"]"
    fi

    cat > "$OUT/node-$i.yaml" <<EOF
node_state_dir: ./state/$TOPO/node-$i
listen_host: 127.0.0.1
listen_port: $port
bootstrap_peers: $peers
node_id_bits: 256
k_bucket_size: 4
alpha: 3
connect_timeout_ms: 3000
read_timeout_ms: 5000
ping_timeout_ms: 5000
max_frame_payload: 65536
protocol_version: 1
log_level: INFO
advertise_host: 127.0.0.1
EOF
done

echo "generated $N configs in $OUT/"