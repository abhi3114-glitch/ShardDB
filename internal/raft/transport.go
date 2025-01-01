package raft

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/raft/v3/raftpb"
)

type HTTPTransport struct {
	mu     sync.RWMutex
	peers  map[uint64]string // ID -> URL (http://ip:port)
	client *http.Client
}

func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{
		peers:  make(map[uint64]string),
		client: &http.Client{Timeout: 1 * time.Second},
	}
}

func (t *HTTPTransport) AddPeer(id uint64, url string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[id] = url
}

func (t *HTTPTransport) SetPeers(peers map[uint64]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers = peers
}

func (t *HTTPTransport) Send(msgs []raftpb.Message) {
	for _, msg := range msgs {
		// Send async to avoid blocking Raft loop (especially if peer is down/slow)
		go func(m raftpb.Message) {
			t.mu.RLock()
			url, ok := t.peers[m.To]
			t.mu.RUnlock()

			if !ok {
				return // Unknown peer
			}

			// Marshal
			data, err := m.Marshal()
			if err != nil {
				log.Printf("Failed to marshal raft msg: %v", err)
				return
			}

			// Post with timeout
			client := http.Client{Timeout: 500 * time.Millisecond}
			// Fix: Append "/raft" endpoint
			fullURL := url
			if !strings.HasSuffix(fullURL, "/raft") {
				fullURL += "/raft"
			}
			resp, err := client.Post(fullURL, "application/octet-stream", bytes.NewReader(data))
			if err != nil {
				// log.Printf("Failed to send to %d (%s): %v", m.To, url, err)
				return
			}
			resp.Body.Close()
		}(msg)
	}
}

// Handler returns an http.Handler that passes requests to the Node.
func (t *HTTPTransport) Handler(node *Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		var msg raftpb.Message
		if err := msg.Unmarshal(data); err != nil {
			http.Error(w, "Invalid protobuf", http.StatusBadRequest)
			return
		}

		// Step the node
		if err := node.Step(r.Context(), msg); err != nil {
			// Step error?
			fmt.Printf("Step error: %v\n", err)
		}
	}
}


