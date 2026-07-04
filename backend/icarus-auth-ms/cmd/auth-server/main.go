package main

import (
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/handler"
	"icarus-auth-ms/internal/repository"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present
	if err := godotenv.Load("env/.env"); err != nil {
		log.Println("No .env file found; using system environment variables")
	}

	log.Println("Starting Auth Microservice...")

	keysDir := "./keys"
	if envKeysDir := os.Getenv("KEYS_DIR"); envKeysDir != "" {
		keysDir = envKeysDir
	}
	privKey, pubKey, err := crypto.LoadOrGenerateKeys(keysDir)
	if err != nil {
		log.Fatalf("Failed to initialize cryptographic keys: %v", err)
	}
	log.Printf("RSA key pair loaded successfully from %s", keysDir)

	dbDriver := "sqlite"
	if envDbDriver := os.Getenv("DATABASE_DRIVER"); envDbDriver != "" {
		dbDriver = envDbDriver
	}

	var dbConnStr string
	if dbDriver == "postgres" {
		if envDbUrl := os.Getenv("DATABASE_URL"); envDbUrl != "" {
			dbConnStr = envDbUrl
		} else {
			log.Fatalf("DATABASE_URL environment variable is required when DATABASE_DRIVER is set to postgres")
		}
	} else {
		dbConnStr = "./auth.db"
		if envDbPath := os.Getenv("DATABASE_PATH"); envDbPath != "" {
			dbConnStr = envDbPath
		}
	}

	dbConn, err := repository.InitDB(dbDriver, dbConnStr)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	if dbDriver == "postgres" {
		log.Printf("Database initialized with driver %s (connection string masked)", dbDriver)
	} else {
		log.Printf("Database initialized with driver %s at %s", dbDriver, dbConnStr)
	}
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn, dbDriver)
	if err := repo.SeedDefaultRBAC(); err != nil {
		log.Fatalf("Failed to seed default RBAC data: %v", err)
	}
	tokenMgr := crypto.NewTokenManager(privKey, pubKey, "icarus-auth-ms")

	server := handler.NewHandlerServer(repo, tokenMgr)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})

	port := "8080"
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
