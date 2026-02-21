#!/usr/bin/env bash
# Mock service for crux playground - runs until Ctrl+C
NAME="${1:-mock}"
echo "Mock service: $NAME (PID $$)"
while true; do
  echo "$(date '+%H:%M:%S') [$NAME] running..."
  sleep 3
done
