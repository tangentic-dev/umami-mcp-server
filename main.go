package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Printf("umami-mcp %s (%s) built %s\n", version, commit, date)
		os.Exit(0)
	}

	transport := os.Getenv("TRANSPORT")
	if transport == "" {
		transport = "stdio"
	}

	switch transport {
	case "http":
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		origins := parseOrigins(os.Getenv("ALLOWED_ORIGINS"))
		maxSessions := 0
		if v := os.Getenv("MAX_SESSIONS"); v != "" {
			maxSessions, _ = strconv.Atoi(v)
		}
		handler := NewHTTPHandler(origins, maxSessions)
		if pinned := os.Getenv("UMAMI_URL"); pinned != "" {
			handler.SetPinnedHost(pinned)
			log.Printf("Pinned to Umami host %s; client-supplied hosts are ignored", pinned)
		}
		ttl := time.Hour
		if v := os.Getenv("SESSION_TTL"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				log.Fatalf("Invalid SESSION_TTL %q: use a Go duration like 30m or 2h", v)
			}
			ttl = d
		}
		handler.StartSessionReaper(ttl)
		mux := http.NewServeMux()
		mux.Handle("/mcp", handler)
		mux.HandleFunc("/.well-known/mcp/server-card.json", handler.handleServerCard)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "/app/index.html")
		})
		srv := &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Printf("Starting HTTP transport on :%s", port)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	default:
		config, err := LoadConfig()
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		var client *UmamiClient
		if config.APIKey != "" {
			client = NewUmamiClientWithAPIKey(config.UmamiURL, config.APIKey)
		} else {
			client = NewUmamiClient(config.UmamiURL, config.Username, config.Password)
		}
		client.teamID = config.TeamID
		if err := client.Authenticate(); err != nil {
			log.Fatalf("Failed to authenticate with Umami: %v", err)
		}

		server := NewMCPServer(client)
		if err := server.Run(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
