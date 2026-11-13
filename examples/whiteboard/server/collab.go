package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CollabServer struct {
	mu       sync.Mutex
	sessions map[string]*collabSession
	rooms    map[string]map[string]bool // roomId -> set of sid
}

type collabSession struct {
	sid      string
	rooms    map[string]bool
	queue    chan string
	lastSeen time.Time
}

func NewCollabServer() *CollabServer {
	cs := &CollabServer{
		sessions: make(map[string]*collabSession),
		rooms:    make(map[string]map[string]bool),
	}
	go cs.reaper()
	return cs
}

func (cs *CollabServer) reaper() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		cs.mu.Lock()
		now := time.Now()
		for sid, sess := range cs.sessions {
			if now.Sub(sess.lastSeen) > 60*time.Second {
				cs.removeSessionLocked(sid)
			}
		}
		cs.mu.Unlock()
	}
}

func (cs *CollabServer) removeSessionLocked(sid string) {
	sess, ok := cs.sessions[sid]
	if !ok {
		return
	}
	delete(cs.sessions, sid)
	close(sess.queue)
	for roomID := range sess.rooms {
		if members, ok := cs.rooms[roomID]; ok {
			delete(members, sid)
			if len(members) == 0 {
				delete(cs.rooms, roomID)
			} else {
				cs.broadcastRoomUserChangeLocked(roomID)
			}
		}
	}
}

func (cs *CollabServer) broadcastRoomUserChangeLocked(roomID string) {
	members, ok := cs.rooms[roomID]
	if !ok {
		return
	}
	sids := make([]string, 0, len(members))
	for s := range members {
		sids = append(sids, s)
	}
	payload, _ := json.Marshal(sids)
	pkt := fmt.Sprintf("42[\"room-user-change\",%s]", string(payload))
	for s := range members {
		if sess, ok := cs.sessions[s]; ok {
			select {
			case sess.queue <- pkt:
			default:
			}
		}
	}
}

func (cs *CollabServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	q := r.URL.Query()
	sid := q.Get("sid")

	if r.Method == http.MethodGet {
		if sid == "" {
			// Handshake: create session
			b := make([]byte, 10)
			_, _ = rand.Read(b)
			newSID := hex.EncodeToString(b)

			sess := &collabSession{
				sid:      newSID,
				rooms:    make(map[string]bool),
				queue:    make(chan string, 128),
				lastSeen: time.Now(),
			}

			cs.mu.Lock()
			cs.sessions[newSID] = sess
			cs.mu.Unlock()

			// Engine.IO handshake response (EIO=4)
			handshake := fmt.Sprintf(`0{"sid":"%s","upgrades":[],"pingInterval":25000,"pingTimeout":20000,"maxPayload":1000000}`, newSID)
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			_, _ = w.Write([]byte(handshake))
			return
		}

		// Polling GET: wait for packets for this session
		cs.mu.Lock()
		sess, ok := cs.sessions[sid]
		if ok {
			sess.lastSeen = time.Now()
		}
		cs.mu.Unlock()

		if !ok {
			http.Error(w, `{"code":1,"message":"Session ID unknown"}`, http.StatusBadRequest)
			return
		}

		select {
		case msg, ok := <-sess.queue:
			if !ok {
				http.Error(w, "session closed", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			_, _ = w.Write([]byte(msg))
		case <-time.After(20 * time.Second):
			// Timeout: send no-op or empty response so client re-polls
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			_, _ = w.Write([]byte("6")) // NOOP in Engine.IO
		}
		return
	}

	if r.Method == http.MethodPost {
		if sid == "" {
			http.Error(w, "missing sid", http.StatusBadRequest)
			return
		}

		cs.mu.Lock()
		sess, ok := cs.sessions[sid]
		if ok {
			sess.lastSeen = time.Now()
		}
		cs.mu.Unlock()

		if !ok {
			http.Error(w, `{"code":1,"message":"Session ID unknown"}`, http.StatusBadRequest)
			return
		}

		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}

		body := string(bodyBytes)
		cs.handlePacket(sess, body)

		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = w.Write([]byte("ok"))
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (cs *CollabServer) handlePacket(sess *collabSession, body string) {
	// Engine.IO v4 packets:
	// '2' = ping -> respond '3' pong
	// '40' = Socket.IO connect
	// '42[...]' = Socket.IO message event
	if body == "2" {
		select {
		case sess.queue <- "3":
		default:
		}
		return
	}

	if body == "40" {
		// Connected
		select {
		case sess.queue <- "40{\"sid\":\"" + sess.sid + "\"}":
		default:
		}
		select {
		case sess.queue <- "42[\"init-room\"]":
		default:
		}
		return
	}

	if strings.HasPrefix(body, "42") {
		jsonPart := body[2:]
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(jsonPart), &arr); err != nil || len(arr) == 0 {
			return
		}
		var eventName string
		if err := json.Unmarshal(arr[0], &eventName); err != nil {
			return
		}

		switch eventName {
		case "join-room":
			if len(arr) > 1 {
				var roomID string
				if err := json.Unmarshal(arr[1], &roomID); err == nil && roomID != "" {
					cs.joinRoom(sess, roomID)
				}
			}
		case "server-broadcast", "server-volatile-broadcast":
			if len(arr) >= 4 {
				var roomID string
				if err := json.Unmarshal(arr[1], &roomID); err == nil && roomID != "" {
					// arr[2] is encryptedData, arr[3] is iv
					pkt := fmt.Sprintf("42[\"client-broadcast\",%s,%s]", string(arr[2]), string(arr[3]))
					cs.broadcastToRoomOthers(sess.sid, roomID, pkt)
				}
			}
		case "user-follow":
			if len(arr) >= 2 {
				// Relayed user follow
				pkt := fmt.Sprintf("42[\"user-follow-room-change\",%s]", string(arr[1]))
				select {
				case sess.queue <- pkt:
				default:
				}
			}
		}
	}
}

func (cs *CollabServer) joinRoom(sess *collabSession, roomID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	sess.rooms[roomID] = true
	members, ok := cs.rooms[roomID]
	if !ok {
		members = make(map[string]bool)
		cs.rooms[roomID] = members
	}
	members[sess.sid] = true

	if len(members) <= 1 {
		select {
		case sess.queue <- "42[\"first-in-room\"]":
		default:
		}
	} else {
		newMsg := fmt.Sprintf("42[\"new-user\",\"%s\"]", sess.sid)
		for s := range members {
			if s != sess.sid {
				if other, ok := cs.sessions[s]; ok {
					select {
					case other.queue <- newMsg:
					default:
					}
				}
			}
		}
	}
	cs.broadcastRoomUserChangeLocked(roomID)
}

func (cs *CollabServer) broadcastToRoomOthers(fromSID, roomID, msg string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	members, ok := cs.rooms[roomID]
	if !ok {
		return
	}
	for s := range members {
		if s != fromSID {
			if other, ok := cs.sessions[s]; ok {
				select {
				case other.queue <- msg:
				default:
				}
			}
		}
	}
}
