#!/bin/bash

# Trap SIGINT (Ctrl+C) and SIGTERM to kill all background jobs
trap 'kill 0' EXIT

echo "Starting SAV Domain services..."

# 1. Run Platform Service
echo "Starting Platform Service (sav-platform-ms)..."
cd "$(dirname "$0")/backend/sav-platform-ms" && go run cmd/platform-server/main.go &

# 2. Run Retire Service
echo "Starting Retire Service (retire-ms)..."
cd "$(dirname "$0")/backend/retire-ms" && go run cmd/retire-server/main.go &

# 3. Run SAV Frontend
echo "Starting SAV Frontend (sav-frontend)..."
cd "$(dirname "$0")/frontend/sav-frontend" && npm start &

# 4. Run SAV Admin Frontend
echo "Starting SAV Admin Frontend (sav-admin-frontend)..."
cd "$(dirname "$0")/frontend/sav-admin-frontend" && npm start &

# Wait for all background jobs to finish
wait
