package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

	"argus/internal/server"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "Listen address")
	agentToken := flag.String("agent-token", os.Getenv("AGENT_TOKEN"), "Legacy shared agent token (fallback)")
	operatorPw := flag.String("operator-password", os.Getenv("OPERATOR_PASSWORD"), "Operator web UI password")
	agentsFile := flag.String("agents-file", os.Getenv("AGENTS_FILE"), "JSON file mapping agent IDs to per-agent tokens")
	flag.Parse()

	if *agentToken == "" || *operatorPw == "" {
		log.Fatal("--agent-token and --operator-password are required (or set AGENT_TOKEN / OPERATOR_PASSWORD env vars)")
	}
	if len(*agentToken) < 32 {
		log.Fatalf("AGENT_TOKEN must be at least 32 bytes (got %d)", len(*agentToken))
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	srv := server.New(*agentToken, *operatorPw, *agentsFile)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux, sub)

	log.Printf("Server listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
