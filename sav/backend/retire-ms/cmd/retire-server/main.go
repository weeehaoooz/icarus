package main

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"retire-ms/internal/handler"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present
	if err := godotenv.Load("env/.env"); err != nil {
		log.Println("No .env file found; using system environment variables")
	}

	log.Println("Starting Retire Microservice...")

	// Register module asynchronously on startup so it doesn't block server listening
	go registerModule()

	server := handler.NewHandlerServer()

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})

	port := "8082"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	log.Printf("Server listening on port %s", port)

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: server.LoggerMiddleware(mux),
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func registerModule() {
	log.Println("Registering module with platform-ms...")

	// 1. Read manifest
	data, err := os.ReadFile("rbac-manifest.json")
	if err != nil {
		log.Printf("Failed to read rbac-manifest.json: %v", err)
		return
	}

	// 2. Resolve platform-ms registration URL
	platformURL := os.Getenv("PLATFORM_MS_REGISTRATION_URL")
	if platformURL == "" {
		platformURL = "http://localhost:8081/api/v1/governance/modules/register"
	}

	// 3. Post to platform-ms
	resp, err := http.Post(platformURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to connect to platform-ms: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Module registration failed with status: %d", resp.StatusCode)
		return
	}

	log.Println("Module successfully registered with platform-ms!")
}
