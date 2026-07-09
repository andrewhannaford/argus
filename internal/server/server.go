package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"argus/internal/protocol"
)

type agent struct {
	info     protocol.AgentInfo
	conn     *websocket.Conn
	mu       sync.Mutex
	operator *operator
}

type operator struct {
	conn  *websocket.Conn
	mu    sync.Mutex
	agent *agent
}

type Server struct {
	agentToken       string
	operatorPassword string
	agents           map[string]*agent
	mu               sync.RWMutex
	upgrader         websocket.Upgrader
}

func New(agentToken, operatorPassword string) *Server {
	return &Server{
		agentToken:       agentToken,
		operatorPassword: operatorPassword,
		agents:           make(map[string]*agent),
		upgrader:         websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux, webFiles fs.FS) {
	mux.Handle("/", http.FileServer(http.FS(webFiles)))
	mux.HandleFunc("/ws/agent", s.handleAgent)
	mux.HandleFunc("/ws/operator", s.handleOperator)
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})

	var msg protocol.Message
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != protocol.MsgAgentAuth {
		return
	}

	var auth protocol.AgentAuthMsg
	if err := json.Unmarshal(msg.Data, &auth); err != nil {
		return
	}
	if auth.Token != s.agentToken {
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "invalid token"})
		return
	}

	a := &agent{
		info: protocol.AgentInfo{ID: auth.AgentID, Hostname: auth.Hostname, OS: auth.OS},
		conn: conn,
	}

	s.mu.Lock()
	s.agents[a.info.ID] = a
	s.mu.Unlock()

	log.Printf("[agent] connected: %s (%s / %s)", a.info.ID, a.info.Hostname, a.info.OS)
	writeMsg(conn, protocol.MsgAuthOK, nil)

	defer func() {
		s.mu.Lock()
		delete(s.agents, a.info.ID)
		s.mu.Unlock()

		a.mu.Lock()
		op := a.operator
		a.mu.Unlock()
		if op != nil {
			op.mu.Lock()
			op.agent = nil
			op.mu.Unlock()
			writeMsg(op.conn, protocol.MsgError, protocol.ErrorMsg{Message: "agent disconnected"})
		}
		log.Printf("[agent] disconnected: %s", a.info.ID)
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		a.mu.Lock()
		op := a.operator
		a.mu.Unlock()

		if op != nil {
			op.mu.Lock()
			op.conn.WriteMessage(websocket.TextMessage, raw)
			op.mu.Unlock()
		}
	}
}

func (s *Server) handleOperator(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})

	var msg protocol.Message
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != protocol.MsgOperatorAuth {
		return
	}

	var auth protocol.OperatorAuthMsg
	if err := json.Unmarshal(msg.Data, &auth); err != nil {
		return
	}
	if auth.Password != s.operatorPassword {
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "invalid password"})
		return
	}

	op := &operator{conn: conn}
	writeMsg(conn, protocol.MsgAuthOK, nil)
	log.Printf("[operator] connected")
	s.sendAgentList(op)

	defer func() {
		op.mu.Lock()
		a := op.agent
		op.mu.Unlock()
		if a != nil {
			a.mu.Lock()
			a.operator = nil
			a.mu.Unlock()
		}
		log.Printf("[operator] disconnected")
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var m protocol.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		switch m.Type {
		case protocol.MsgAgentList:
			s.sendAgentList(op)

		case protocol.MsgSelectAgent:
			var sel protocol.SelectAgentMsg
			if err := json.Unmarshal(m.Data, &sel); err != nil {
				continue
			}
			s.mu.RLock()
			a := s.agents[sel.AgentID]
			s.mu.RUnlock()
			if a == nil {
				writeMsg(conn, protocol.MsgError, protocol.ErrorMsg{Message: "agent not found"})
				continue
			}
			// detach from previous agent
			op.mu.Lock()
			if op.agent != nil {
				op.agent.mu.Lock()
				op.agent.operator = nil
				op.agent.mu.Unlock()
			}
			op.agent = a
			op.mu.Unlock()
			// attach to new agent
			a.mu.Lock()
			a.operator = op
			a.mu.Unlock()

		default:
			op.mu.Lock()
			a := op.agent
			op.mu.Unlock()
			if a != nil {
				a.mu.Lock()
				a.conn.WriteMessage(websocket.TextMessage, raw)
				a.mu.Unlock()
			}
		}
	}
}

func (s *Server) sendAgentList(op *operator) {
	s.mu.RLock()
	list := make([]protocol.AgentInfo, 0, len(s.agents))
	for _, a := range s.agents {
		list = append(list, a.info)
	}
	s.mu.RUnlock()

	data, _ := json.Marshal(list)
	op.mu.Lock()
	writeMsg(op.conn, protocol.MsgAgentList, json.RawMessage(data))
	op.mu.Unlock()
}

func writeMsg(conn *websocket.Conn, t protocol.MsgType, data interface{}) {
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	b, _ := json.Marshal(protocol.Message{Type: t, Data: raw})
	conn.WriteMessage(websocket.TextMessage, b)
}
