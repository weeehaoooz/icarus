package main

import (
	"log"
	"net/http"
	"os"
	"icarus-admin-ms/internal/crypto"
	"icarus-admin-ms/internal/handler"
	"icarus-admin-ms/internal/repository"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	// Load env file
	if err := godotenv.Load("env/.env"); err != nil {
		log.Println("No .env file found; using default configuration")
	}

	log.Println("Starting Platform Microservice...")

	dbPath := "./platform.db"
	if envDbPath := os.Getenv("DATABASE_PATH"); envDbPath != "" {
		dbPath = envDbPath
	}

	dbConn, err := repository.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Printf("Database initialized at %s", dbPath)
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn)

	authMsCertsURL := "http://localhost:8080/certs?format=pem"
	if envCertsURL := os.Getenv("AUTH_MS_CERTS_URL"); envCertsURL != "" {
		authMsCertsURL = envCertsURL
	}
	verifier := crypto.NewTokenVerifier(authMsCertsURL)

	authMsBaseURL := "http://localhost:8080"
	if idx := strings.Index(authMsCertsURL, "/certs"); idx != -1 {
		authMsBaseURL = authMsCertsURL[:idx]
	}

	server := handler.NewHandlerServer(repo, verifier, authMsBaseURL)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})

	port := "8081"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	log.Printf("Server listening on port %s", port)

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: server.CORSMiddleware(server.LoggerMiddleware(mux)),
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
