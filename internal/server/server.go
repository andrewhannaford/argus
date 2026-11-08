package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"argus/internal/protocol"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

const (
	pingPeriod     = 30 * time.Second
	pongWait       = 60 * time.Second
	maxMessageSize = 32 << 20
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
	ip    string
}

type Server struct {
	agentToken       string
	operatorPassword string
	agents           map[string]*agent
	operators        map[*operator]struct{}
	mu               sync.RWMutex
	upgrader         websocket.Upgrader
	limiter          *authLimiter
}

func New(agentToken, operatorPassword string) *Server {
	return &Server{
		agentToken:       agentToken,
		operatorPassword: operatorPassword,
		agents:           make(map[string]*agent),
		operators:        make(map[*operator]struct{}),
		upgrader:         websocket.Upgrader{CheckOrigin: checkOrigin},
		limiter:          newAuthLimiter(),
	}
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients (the agent) send no Origin header.
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func secureEqual(a, b string) bool {
	ah := sha256.Sum256([]byte(a))
	bh := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ah[:], bh[:]) == 1
}

// passwordMatches accepts a bcrypt hash (preferred) or a legacy plaintext value.
func passwordMatches(submitted, stored string) bool {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(submitted)) == nil
	}
	return secureEqual(submitted, stored)
}

type authFailure struct {
	count       int
	lockedUntil time.Time
}

type authLimiter struct {
	mu       sync.Mutex
	failures map[string]*authFailure
}

const (
	authMaxFailures = 5
	authLockBase    = 30 * time.Second
	authLockMax     = 15 * time.Minute
	authFailDelay   = 250 * time.Millisecond
)

func newAuthLimiter() *authLimiter {
	return &authLimiter{failures: make(map[string]*authFailure)}
}

func (l *authLimiter) locked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.failures[key]
	if f == nil || f.lockedUntil.IsZero() {
		return false
	}
	if time.Now().After(f.lockedUntil) {
		delete(l.failures, key)
		return false
	}
	return true
}

func (l *authLimiter) recordFail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.failures[key]
	if f == nil {
		f = &authFailure{}
		l.failures[key] = f
	}
	f.count++
	if f.count >= authMaxFailures {
		d := authLockBase << uint(f.count-authMaxFailures)
		if d > authLockMax {
			d = authLockMax
		}
		f.lockedUntil = time.Now().Add(d)
	}
}

func (l *authLimiter) recordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
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
	conn.SetReadLimit(maxMessageSize)

	key := clientIP(r)
	if s.limiter.locked(key) {
		return
	}

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
	if auth.Version != protocol.ProtocolVersion {
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "protocol version mismatch"})
		return
	}
	if !secureEqual(auth.Token, s.agentToken) {
		s.limiter.recordFail(key)
		time.Sleep(authFailDelay)
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "invalid token"})
		return
	}
	if len(auth.AgentID) > 128 || len(auth.Hostname) > 256 || len(auth.OS) > 32 {
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "invalid agent info"})
		return
	}
	s.limiter.recordSuccess(key)

	a := &agent{
		info: protocol.AgentInfo{ID: auth.AgentID, Hostname: auth.Hostname, OS: auth.OS},
		conn: conn,
	}

	s.mu.Lock()
	if old, ok := s.agents[a.info.ID]; ok && old != a {
		old.conn.Close()
	}
	s.agents[a.info.ID] = a
	s.mu.Unlock()

	log.Printf("[agent] connected: %s (%s / %s)", a.info.ID, a.info.Hostname, a.info.OS)
	writeMsg(conn, protocol.MsgAuthOK, nil)
	s.broadcastAgentList()

	done := make(chan struct{})
	defer close(done)

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.mu.Lock()
				conn.WriteMessage(websocket.PingMessage, nil)
				a.mu.Unlock()
			}
		}
	}()

	defer func() {
		s.mu.Lock()
		if s.agents[a.info.ID] == a {
			delete(s.agents, a.info.ID)
		}
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
		s.broadcastAgentList()
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
	conn.SetReadLimit(maxMessageSize)

	key := clientIP(r)
	if s.limiter.locked(key) {
		return
	}

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
	if auth.Version != protocol.ProtocolVersion {
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "protocol version mismatch"})
		return
	}
	if !passwordMatches(auth.Password, s.operatorPassword) {
		s.limiter.recordFail(key)
		time.Sleep(authFailDelay)
		writeMsg(conn, protocol.MsgAuthFail, protocol.ErrorMsg{Message: "invalid password"})
		return
	}
	s.limiter.recordSuccess(key)

	op := &operator{conn: conn, ip: key}
	writeMsg(conn, protocol.MsgAuthOK, nil)
	log.Printf("[operator] connected from %s", key)
	s.sendAgentList(op)

	s.mu.Lock()
	s.operators[op] = struct{}{}
	s.mu.Unlock()

	done := make(chan struct{})
	defer close(done)

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				op.mu.Lock()
				conn.WriteMessage(websocket.PingMessage, nil)
				op.mu.Unlock()
			}
		}
	}()

	defer func() {
		s.mu.Lock()
		delete(s.operators, op)
		s.mu.Unlock()

		op.mu.Lock()
		a := op.agent
		op.mu.Unlock()
		if a != nil {
			a.mu.Lock()
			a.operator = nil
			a.mu.Unlock()
		}
		log.Printf("[operator] disconnected from %s", key)
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
			log.Printf("[audit] %s selected agent %s", op.ip, a.info.ID)

		default:
			op.mu.Lock()
			a := op.agent
			op.mu.Unlock()
			if a != nil {
				switch m.Type {
				case protocol.MsgShellStart:
					log.Printf("[audit] %s started shell on agent %s", op.ip, a.info.ID)
				case protocol.MsgScreenshotReq:
					log.Printf("[audit] %s requested screenshot on agent %s", op.ip, a.info.ID)
				case protocol.MsgCameraStart:
					log.Printf("[audit] %s started camera on agent %s", op.ip, a.info.ID)
				case protocol.MsgCameraStop:
					log.Printf("[audit] %s stopped camera on agent %s", op.ip, a.info.ID)
				}
				a.mu.Lock()
				a.conn.WriteMessage(websocket.TextMessage, raw)
				a.mu.Unlock()
			}
		}
	}
}

func (s *Server) broadcastAgentList() {
	s.mu.RLock()
	ops := make([]*operator, 0, len(s.operators))
	for op := range s.operators {
		ops = append(ops, op)
	}
	s.mu.RUnlock()

	for _, op := range ops {
		s.sendAgentList(op)
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
