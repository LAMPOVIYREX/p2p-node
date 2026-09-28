#!/usr/bin/env bash
mkdir -p configs
for i in $(seq -w 1 21); do
  port=$((9100 + 10#$i))
  boot="[]"
  if [ "$i" != "01" ]; then
    boot="[\"127.0.0.1:9101\"]"   # star
  fi
  cat > configs/node-$i.yaml <<EOF
node_state_dir: ./state/node-$i
listen_host: 127.0.0.1
listen_port: $port
bootstrap_peers: $boot
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