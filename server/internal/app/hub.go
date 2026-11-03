package app

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Peer is one open socket with its own outgoing queue. A peer that cannot
// keep up is disconnected rather than allowed to block the sender.
type Peer struct {
	conn    *websocket.Conn
	queue   chan Frame
	ctx     context.Context
	cancel  context.CancelFunc
	ID      string // Device ID, for screens.
	ClassID string // For screens.
	UserID  string // For teachers.
}

func newPeer(parent context.Context, conn *websocket.Conn) *Peer {
	ctx, cancel := context.WithCancel(parent)
	return &Peer{conn: conn, queue: make(chan Frame, 256), ctx: ctx, cancel: cancel}
}

func (p *Peer) close() {
	p.cancel()
	_ = p.conn.CloseNow()
}

// send queues the frame without waiting.
func (p *Peer) send(f Frame) {
	if p.ctx.Err() != nil {
		return
	}
	select {
	case p.queue <- f:
	default:
		p.close()
	}
}

// enqueue waits for room in the queue and reports whether the frame fit.
func (p *Peer) enqueue(f Frame) bool {
	select {
	case p.queue <- f:
		return true
	case <-p.ctx.Done():
	case <-time.After(writeTimeout):
	}
	return false
}

func (p *Peer) write() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case f := <-p.queue:
			ctx, cancel := context.WithTimeout(p.ctx, writeTimeout)
			err := wsjson.Write(ctx, p.conn, f)
			cancel()
			if err != nil {
				p.close()
				return
			}
		}
	}
}

// Hub tracks one socket per screen and any number per teacher.
type Hub struct {
	mu       sync.Mutex
	devices  map[string]*Peer
	teachers map[string]map[*Peer]bool
}

func NewHub() *Hub { return &Hub{devices: map[string]*Peer{}, teachers: map[string]map[*Peer]bool{}} }

func (h *Hub) DeviceOnline(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.devices[id]
	return p != nil && p.ctx.Err() == nil
}

func (h *Hub) DeviceCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.devices)
}

// addDevice registers the screen's socket and closes the one it replaces.
func (h *Hub) addDevice(p *Peer) {
	h.mu.Lock()
	old := h.devices[p.ID]
	h.devices[p.ID] = p
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
}

// removeDevicePeer forgets the socket if it is still the screen's current one.
func (h *Hub) removeDevicePeer(p *Peer) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.devices[p.ID] != p {
		return false
	}
	delete(h.devices, p.ID)
	return true
}

func (h *Hub) addTeacher(p *Peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.teachers[p.UserID] == nil {
		h.teachers[p.UserID] = map[*Peer]bool{}
	}
	h.teachers[p.UserID][p] = true
}

func (h *Hub) removeTeacherPeer(p *Peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.teachers[p.UserID], p)
	if len(h.teachers[p.UserID]) == 0 {
		delete(h.teachers, p.UserID)
	}
}

func (h *Hub) Device(id string, f Frame) {
	h.mu.Lock()
	p := h.devices[id]
	h.mu.Unlock()
	if p != nil {
		p.send(f)
	}
}

func (h *Hub) Class(id string, f Frame) {
	h.mu.Lock()
	var peers []*Peer
	for _, p := range h.devices {
		if p.ClassID == id {
			peers = append(peers, p)
		}
	}
	h.mu.Unlock()
	sendAll(peers, f)
}

func (h *Hub) Teacher(id string, f Frame) {
	h.mu.Lock()
	var peers []*Peer
	for p := range h.teachers[id] {
		peers = append(peers, p)
	}
	h.mu.Unlock()
	sendAll(peers, f)
}

// sendAll runs after the hub lock is released, because send may close a peer.
func sendAll(peers []*Peer, f Frame) {
	for _, p := range peers {
		p.send(f)
	}
}

func (h *Hub) RemoveDevice(id string) {
	h.mu.Lock()
	p := h.devices[id]
	delete(h.devices, id)
	h.mu.Unlock()
	if p != nil {
		p.close()
	}
}

func (h *Hub) RemoveTeacher(id string) {
	h.mu.Lock()
	peers := h.teachers[id]
	delete(h.teachers, id)
	h.mu.Unlock()
	for p := range peers {
		p.close()
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.devices {
		p.close()
	}
	for _, peers := range h.teachers {
		for p := range peers {
			p.close()
		}
	}
	h.devices = map[string]*Peer{}
	h.teachers = map[string]map[*Peer]bool{}
}
