package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"netdiagram/internal/handler"
	"netdiagram/internal/hub"
	"netdiagram/internal/repository/sqlite"
	"netdiagram/internal/service"
	"netdiagram/internal/watcher"
)

//go:embed web/*
var webFS embed.FS

func main() {
	// Command line flags
	addr := flag.String("addr", ":3000", "HTTP listen address")
	dbPath := flag.String("db", "./netdiagram.db", "SQLite database path")
	yamlPath := flag.String("yaml", "", "Path to infrastructure.yml to import on startup and watch")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Network Diagram server...")

	// Initialize SQLite repository
	repo, err := sqlite.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer repo.Close()
	log.Printf("Database opened: %s", *dbPath)

	// Initialize event bus
	eventBus := service.NewEventBus()

	// Initialize SSE hub
	sseHub := hub.New()
	go sseHub.Run()

	// Connect event bus to SSE hub
	eventChan := make(chan service.Event, 100)
	eventBus.Subscribe(eventChan)
	go func() {
		for event := range eventChan {
			sseHub.Broadcast(event)
		}
	}()

	// Initialize services
	infraSvc := service.NewInfrastructureService(repo, eventBus)

	// Import YAML if specified
	if *yamlPath != "" {
		log.Printf("Importing infrastructure from: %s", *yamlPath)
		if err := infraSvc.ImportFromYAML(context.Background(), *yamlPath); err != nil {
			log.Printf("Warning: Failed to import YAML: %v", err)
		} else {
			log.Println("Infrastructure imported successfully")
		}
	}

	// Initialize HTTP handlers
	apiHandler := handler.NewAPIHandler(infraSvc)

	// Setup routes
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/graph", apiHandler.GetGraph)
	mux.HandleFunc("GET /api/infrastructure", apiHandler.GetInfrastructure)
	mux.HandleFunc("GET /api/hosts/{id}", apiHandler.GetHost)
	mux.HandleFunc("POST /api/hosts", apiHandler.CreateHost)
	mux.HandleFunc("PUT /api/hosts/{id}", apiHandler.UpdateHost)
	mux.HandleFunc("DELETE /api/hosts/{id}", apiHandler.DeleteHost)
	mux.HandleFunc("POST /api/connections", apiHandler.CreateConnection)
	mux.HandleFunc("DELETE /api/connections/{id}", apiHandler.DeleteConnection)
	mux.HandleFunc("POST /api/positions", apiHandler.SavePositions)
	mux.HandleFunc("GET /api/export", apiHandler.ExportYAML)
	mux.HandleFunc("POST /api/import", apiHandler.ImportYAML)
	mux.HandleFunc("POST /api/reload", apiHandler.Reload)
	mux.Handle("GET /api/events", sseHub)

	// Static files from embedded filesystem
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to get embedded web content: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	// Apply middleware
	finalHandler := handler.Chain(mux,
		handler.Recover,
		handler.CORS,
		handler.Logger,
	)

	// Create server
	server := &http.Server{
		Addr:         *addr,
		Handler:      finalHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start file watcher if YAML path specified
	var watcherCancel context.CancelFunc
	if *yamlPath != "" {
		var watcherCtx context.Context
		watcherCtx, watcherCancel = context.WithCancel(context.Background())

		w := watcher.New(*yamlPath, func() {
			log.Println("Infrastructure file changed, reloading...")
			if err := infraSvc.ImportFromYAML(context.Background(), *yamlPath); err != nil {
				log.Printf("Failed to reload YAML: %v", err)
			} else {
				log.Println("Infrastructure reloaded successfully")
			}
		})

		go func() {
			if err := w.Watch(watcherCtx); err != nil && err != context.Canceled {
				log.Printf("Watcher error: %v", err)
			}
		}()
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on %s", *addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Cancel watcher
	if watcherCancel != nil {
		watcherCancel()
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
