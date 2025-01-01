package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/myuser/sharddb/internal/meta"
	"github.com/myuser/sharddb/internal/raft"
)

func main() {
	id := flag.Uint64("id", 10, "Meta Node ID (default 10)")                             // avoid conflict with shard 1,2,3
	cluster := flag.String("cluster", "10=http://meta-node:9000", "Meta Cluster config") // Single node meta for MVP
	port := flag.Int("port", 9000, "Port to listen on")
	flag.Parse()

	fmt.Printf("Starting ShardDB Meta Node ID=%d Port=%d\n", *id, *port)

	// 1. Meta Store (Applier)
	store := meta.NewMetaStore()

	// 2. Transport
	transport := raft.NewHTTPTransport()

	peers := []uint64{}
	parts := strings.Split(*cluster, ",")
	for _, p := range parts {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			continue
		}
		var pid uint64
		fmt.Sscanf(kv[0], "%d", &pid)
		peers = append(peers, pid)
		transport.AddPeer(pid, kv[1])
	}

	// 3. Raft Node
	// Meta node needs persistence too ideally, but MVP memory for now?
	// Let's use DiskStorage if we want consistency across restarts.
	// We'll use "data/meta-wal" path.
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatal(err)
	}
	walPath := fmt.Sprintf("data/meta-wal-%d.log", *id)

	cfg := raft.Config{
		ID:      *id,
		Peers:   peers,
		WALPath: walPath,
	}

	node, err := raft.NewNode(cfg, store, transport)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go node.Run(ctx)

	// 4. HTTP API
	// Raft
	http.HandleFunc("/raft", transport.Handler(node))

	// Public API
	http.HandleFunc("/route", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		shardID := store.GetShard([]byte(key))
		// Return Shard Info?
		// MVP: just return ID + address placeholder?
		// We need to know Shard Addresses.
		// For now simple JSON
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":      key,
			"shard_id": shardID,
			// "address": store.Shards[shardID].Addrs[0]? potentially
		})
	})

	srv := &http.Server{Addr: fmt.Sprintf(":%d", *port)}
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Listen failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	srv.Shutdown(context.Background())
}



