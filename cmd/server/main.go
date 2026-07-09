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
	agentToken := flag.String("agent-token", os.Getenv("AGENT_TOKEN"), "Token agents use to authenticate")
	operatorPw := flag.String("operator-password", os.Getenv("OPERATOR_PASSWORD"), "Operator web UI password")
	flag.Parse()

	if *agentToken == "" || *operatorPw == "" {
		log.Fatal("--agent-token and --operator-password are required (or set AGENT_TOKEN / OPERATOR_PASSWORD env vars)")
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	srv := server.New(*agentToken, *operatorPw)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux, sub)

	log.Printf("Server listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
