# ShardDB

![Go Version](https://img.shields.io/badge/go-1.23+-blue.svg)
![License](https://img.shields.io/badge/license-MIT-green.svg)
![Status](https://img.shields.io/badge/status-production--ready-success)

**ShardDB** is a high-performance, distributed key-value store built in Go, featuring **Strong Consistency**, **Horizontal Scalability**, and **ACID Transactions**. It leverages the [Raft consensus algorithm](https://raft.github.io/) for log replication and handles automatic sharding via a dedicated Metadata coordination layer.

---

## Key Features

*   **Distributed Architecture**: Decoupled Compute (Proxy), Storage (Shards), and Metadata (Meta-Node) layers.
*   **Strong Consistency**: Uses `etcd/raft` to guarantee linearizability at the shard level.
*   **Range-Based Sharding**: Dynamic data distribution allows efficient range scans and automatic load balancing.
*   **ACID Transactions**: Supports distributed cross-shard transactions using **Two-Phase Commit (2PC)** with Optimistic Concurrency Control.
*   **MVCC Storage Engine**: Custom in-memory B-Tree engine with Multi-Version Concurrency Control (MVCC) for non-blocking reads.
*   **SQL-like Interface**: Supports `INSERT`, `SELECT`, `UPDATE`, `DELETE` via a custom SQL parser and execution planner.
*   **Observability**: Built-in Prometheus-compatible metrics endpoints and structured logging.

---

## Architecture

The system consists of three core components:

### 1. The Proxy (Compute Layer)
Stateless gateway that accepts client requests. It fetches the latest **Routing Table** (Range Map) from the Meta-Node and orchestrates queries:
*   **Point Queries**: Routed directly to the owning shard.
*   **Scatter-Gather**: Broadcasts `SELECT *` queries to all shards and merges results.
*   **Transaction Coordinator**: Manages the lifecycle of distributed transactions (Prepare/Commit/Abort).

### 2. The Meta-Node (Control Plane)
The source of truth for cluster topology.
*   Maintains the **Range Map** (`[KeyStart, KeyEnd) -> ShardID`).
*   Handles Shard Registration and Health checks (implied via Raft group membership).
*   Backed by its own Raft group for high availability.

### 3. Shard Nodes (Data Plane)
The workhorses that store data.
*   **Raft Group**: Each Shard is a 3-node Raft cluster.
*   **Storage Engine**: In-memory B-Tree + Write-Ahead Log (WAL) for durability.
*   **Replication**: Entries are committed only after a quorum (2/3) acknowledges them.

---

## Getting Started

### Prerequisites
*   Go 1.23 or higher.
*   PowerShell (for the startup script) or Bash (simple adaptation required).

### Installation

1.  Clone the repository:
    ```bash
    git clone https://github.com/abhi3114-glitch/ShardDB.git
    cd ShardDB
    ```

2.  Start the Cluster (3 Shards, 1 Proxy, 1 Meta):
    ```powershell
    .\start_cluster.ps1
    ```
    *This script builds all binaries (`bin/`) and launches them with the correct port configurations and log redirection.*

---

## API Reference

The Proxy exposes a RESTful HTTP API on `http://localhost:8080`.

### Execute Query
Execute a SQL command against the cluster.

**Endpoint**: `POST /execute`
**Query Param**: `sql` (The SQL statement)

**Examples**:

*   **Write Data**:
    ```bash
    curl "http://localhost:8080/execute?sql=INSERT+INTO+users+VALUES+('u1','Alice')"
    ```

*   **Read Data**:
    ```bash
    curl "http://localhost:8080/execute?sql=SELECT+*+FROM+users+WHERE+k='u1'"
    ```
    *Response*: `[["u1","Alice"]]`

*   **Range Scan**:
    ```bash
    curl "http://localhost:8080/execute?sql=SELECT+*+FROM+users+WHERE+k>'u1'"
    ```

### Metrics
Prometheus-formatted metrics for monitoring.

**Endpoint**: `GET /metrics` *(Available on all nodes)*
**Ports**:
*   Proxy: `:8080`
*   Meta: `:9000`
*   Shards: `:9001`, `:9002`, `:9003`

---

## Internals & Development

### Directory Structure

| Path | Description |
| :--- | :--- |
| `cmd/` | Entry points for system components (`shard-node`, `proxy`, `meta-node`). |
| `internal/raft` | Raft implementation wrapping `etcd/raft`. Handles Transport, WAL, and Snapshots. |
| `internal/storage` | The Storage Engine. Implements the B-Tree, MVCC encoding, and GC. |
| `internal/txn` | 2PC Coordinator and distributed transaction logic. |
| `internal/sql` | Query Parser and Logical Planner. |
| `docs/` | Detailed architecture documentation (`implementation_plan.md`, `walkthrough.md`). |

### Benchmarking
A stress-test tool is included to verify performance and concurrency.

```bash
# Run with 10 concurrent workers for 10 seconds
go run cmd/tools/benchmark/main.go -concurrency=10 -duration=10s
```
**Expected Performance**: ~750+ RPS (Single-Node Dev Environment).

---

## Durability & Persistence

Data is **persisted to disk** in the `data/` directory.
*   **WAL**: Every Raft entry is written to a Write-Ahead Log before being applied.
*   **Snapshots**: Periodic snapshots compress the log history to save space and speed up recovery.

If a node crashes, it restarts and replays the WAL to restore its state.

---

## Contributing

This project is open-source. Please see `docs/task.md` for the development checklist and `docs/implementation_plan.md` for the original design specification.

**Author**: Abhi
**License**: MIT




