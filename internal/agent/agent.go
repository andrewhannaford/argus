package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"argus/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	pingPeriod     = 30 * time.Second
	pongWait       = 60 * time.Second
	maxMessageSize = 32 << 20
)

type Agent struct {
	serverURL string
	token     string
	id        string
	conn      *websocket.Conn
	connMu    sync.Mutex
	sendMu    sync.Mutex // gorilla/websocket requires serialized writes
	stopped   bool
	shell     *Shell
	camera    *Camera
}

func New(serverURL, token, id string) *Agent {
	return &Agent{serverURL: serverURL, token: token, id: id}
}

func (a *Agent) Run() error {
	hostname, _ := os.Hostname()
	for {
		a.connMu.Lock()
		if a.stopped {
			a.connMu.Unlock()
			return nil
		}
		a.connMu.Unlock()

		if err := a.connect(hostname); err != nil {
			log.Printf("connection error: %v — reconnecting in 5s", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// Stop signals the agent to exit its run loop. It closes the active WebSocket
// connection so the blocked ReadMessage call returns immediately.
func (a *Agent) Stop() {
	a.connMu.Lock()
	a.stopped = true
	if a.conn != nil {
		a.conn.Close()
	}
	a.connMu.Unlock()
}

func (a *Agent) connect(hostname string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(a.serverURL+"/ws/agent", nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetReadLimit(maxMessageSize)

	a.connMu.Lock()
	a.conn = conn
	a.connMu.Unlock()

	if err := a.send(protocol.MsgAgentAuth, protocol.AgentAuthMsg{
		Version: protocol.ProtocolVersion, Token: a.token, AgentID: a.id, Hostname: hostname, OS: runtime.GOOS,
	}); err != nil {
		return err
	}

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	var msg protocol.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	if msg.Type == protocol.MsgAuthFail {
		var e protocol.ErrorMsg
		if json.Unmarshal(msg.Data, &e) == nil {
			return fmt.Errorf("authentication failed: %s", e.Message)
		}
		return fmt.Errorf("authentication failed")
	}
	if msg.Type != protocol.MsgAuthOK {
		return fmt.Errorf("unexpected auth response: %s", msg.Type)
	}
	log.Printf("Connected to server as %s", a.id)

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.sendMu.Lock()
				conn.WriteMessage(websocket.PingMessage, nil)
				a.sendMu.Unlock()
			}
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var m protocol.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		a.handle(m)
	}
}

func (a *Agent) handle(m protocol.Message) {
	switch m.Type {
	case protocol.MsgShellStart:
		if a.shell != nil {
			a.shell.Close()
		}
		a.shell = NewShell(func(data string) {
			a.send(protocol.MsgShellOutput, protocol.ShellOutputMsg{Data: data})
		})
		if err := a.shell.Start(); err != nil {
			log.Printf("shell start error: %v", err)
			a.shell = nil
		}

	case protocol.MsgShellInput:
		if a.shell != nil {
			var inp protocol.ShellInputMsg
			if err := json.Unmarshal(m.Data, &inp); err == nil {
				a.shell.Write(inp.Data)
			}
		}

	case protocol.MsgShellResize:
		if a.shell != nil {
			var r protocol.ShellResizeMsg
			if err := json.Unmarshal(m.Data, &r); err == nil {
				a.shell.Resize(r.Cols, r.Rows)
			}
		}

	case protocol.MsgScreenshotReq:
		display := 0
		var req protocol.ScreenshotReqMsg
		if json.Unmarshal(m.Data, &req) == nil {
			display = req.Display
		}
		go func() {
			b64, err := captureScreenshot(display)
			if err != nil {
				log.Printf("screenshot error: %v", err)
				return
			}
			a.send(protocol.MsgScreenshotData, protocol.ScreenshotDataMsg{Image: b64})
		}()

	case protocol.MsgCameraStart:
		if a.camera != nil {
			a.camera.Stop()
		}
		a.camera = NewCamera(func(frame string) {
			a.send(protocol.MsgCameraFrame, protocol.CameraFrameMsg{Image: frame})
		})
		go a.camera.Start()

	case protocol.MsgCameraStop:
		if a.camera != nil {
			a.camera.Stop()
			a.camera = nil
		}
	}
}

func (a *Agent) send(t protocol.MsgType, data interface{}) error {
	raw, _ := json.Marshal(data)
	b, _ := json.Marshal(protocol.Message{Type: t, Data: raw})
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	return a.conn.WriteMessage(websocket.TextMessage, b)
}
