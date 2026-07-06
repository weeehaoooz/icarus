#!/bin/bash

# Trap SIGINT (Ctrl+C) and SIGTERM to kill all background jobs
trap 'kill 0' EXIT

echo "Starting Icarus Domain services..."

# 1. Run Auth Service
echo "Starting Auth Service (icarus-auth-ms)..."
cd "$(dirname "$0")/backend/icarus-auth-ms" && go run cmd/auth-server/main.go &

# 2. Run Admin Service
echo "Starting Admin Service (icarus-admin-ms)..."
cd "$(dirname "$0")/backend/icarus-admin-ms" && go run cmd/admin-server/main.go &

# 3. Run Workflow Service
echo "Starting Workflow Service (icarus-workflow-ms)..."
cd "$(dirname "$0")/backend/icarus-workflow-ms" && go run cmd/workflow-server/main.go &

# 4. Run Admin Frontend
echo "Starting Admin Frontend (icarus-frontend)..."
cd "$(dirname "$0")/frontend/icarus-frontend" && npm start &

# Wait for all background jobs to finish
wait

