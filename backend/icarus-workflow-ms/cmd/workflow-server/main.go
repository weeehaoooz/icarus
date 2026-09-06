package main

import (
	"icarus-workflow-ms/internal/crypto"
	"icarus-workflow-ms/internal/handler"
	"icarus-workflow-ms/internal/repository"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("env/.env"); err != nil {
		log.Println("No .env file found; using default configuration")
	}

	log.Println("Starting Workflow Microservice...")

	dbPath := "./workflow.db"
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		dbPath = v
	}

	dbConn, err := repository.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Printf("Database initialized at %s", dbPath)
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn)

	authMsCertsURL := "http://localhost:8080/api/v1/certs?format=pem"
	if v := os.Getenv("AUTH_MS_CERTS_URL"); v != "" {
		authMsCertsURL = v
	}
	verifier := crypto.NewTokenVerifier(authMsCertsURL)

	adminMSURL := "http://localhost:8081"
	if v := os.Getenv("ADMIN_MS_URL"); v != "" {
		adminMSURL = v
	}
	_ = strings.TrimRight(adminMSURL, "/")

	srv := handler.NewHandlerServer(repo, verifier, adminMSURL)
	defer srv.SecLogger.Close()

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"icarus-workflow-ms"}`))
	})

	port := "8082"
	if v := os.Getenv("PORT"); v != "" {
		port = v
	}
	log.Printf("Workflow service listening on port %s", port)

	httpSrv := &http.Server{
		Addr:    ":" + port,
		Handler: srv.CORSMiddleware(srv.LoggerMiddleware(mux)),
	}
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
