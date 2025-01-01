# Implementation Plan - ShardDB (Phase 0 & 1)

**Goal:** Initialize the project and build the core single-node storage engine with SQL support and MVCC.

## User Review Required
None for this initial setup phase.

## Proposed Changes

### Project Setup
#### [NEW] [go.mod](file:///c:/PROJECTS/GOD/ShardDB/go.mod)
- Initialize module `github.com/myuser/sharddb` (or similar).

#### [NEW] [cmd/shard-node/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- Entry point for the shard server.

#### [NEW] [cmd/proxy/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/proxy/main.go)
- Entry point for the SQL proxy.

#### [NEW] [docker/compose.dev.yaml](file:///c:/PROJECTS/GOD/ShardDB/docker/compose.dev.yaml)
- Define services for 3 shard nodes and 1 proxy.

#### [NEW] [internal/storage/engine.go](file:///c:/PROJECTS/GOD/ShardDB/internal/storage/engine.go)
- Define `Engine` interface and basic in-memory implementation.
- Basic Put/Get methods.
- [x] Define `Engine` interface and basic in-memory implementation.
- [x] Basic Put/Get methods.

### SQL & Planner (Initial)
#### [NEW] [internal/sql/parser.go](file:///c:/PROJECTS/GOD/ShardDB/internal/sql/parser.go)
- [x] Basic wrapper around a parser library (e.g., `github.com/blastrain/vitess-sqlparser` or `pingcap/parser` if feasible, or write simple one). *Decision: Start with a simple hand-rolled parser or lightweight library for MVP to avoid huge dependencies if possible, or use `x/tools` logic. User suggested PEG or ANTLR, or handwritten. I'll stick to a simple parser initially for basic commands.* -> *Update: Let's use `github.com/blastrain/vitess-sqlparser` for robustness if needed, but for MVP "No full SQL standard" helps.*

## Verification Plan

### Automated Tests
- [x] Reuse `verify_proxy_2pc.go`.
- [x] Run `go test ./...` after implementing storage.
- [x] Create a test ensuring docker compose up brings up the services (health check).

### Manual Verification
- [x] Run `docker-compose up` and check logs.

## Phase 2: Raft Integration
**Goal:** Implement distributed consensus using `etcd/raft` to replicate the State Machine (KV Store) across 3 nodes.

### Proposed Changes
#### [NEW] [internal/raft/node.go](file:///c:/PROJECTS/GOD/ShardDB/internal/raft/node.go)
- Handles the run loop: ticking, processing `Ready` channel (entries, messages, snapshot).
- Integrates `raft.Storage` (MemoryStorage).

#### [NEW] [internal/raft/transport.go](file:///c:/PROJECTS/GOD/ShardDB/internal/raft/transport.go)
- Simple HTTP transport to send `raftpb.Message` to peers.

#### [MODIFY] [cmd/shard-node/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- Initialize `RaftNode`.
- Start HTTP server for internal Raft traffic.
- Wiring: API Request -> Raft Propose -> Commit -> Apply to Engine.

### Verification
- **Test:** `TestRaftNode` (unit).
- **Integration:** Start 3-node cluster, write to Leader, verify Get from Follower (after apply).

## Phase 3: Meta-Shard & RangeMap
**Goal:** Implement the Meta-Shard to manage cluster topology and data ranges, and a Proxy to route client requests.

### Proposed Changes
#### [NEW] [internal/meta/store.go](file:///c:/PROJECTS/GOD/ShardDB/internal/meta/store.go)
- `RangeMap` struct: Sorted list of `Range` {StartKey, EndKey, ShardIDs}.
- `MetaStore` struct: Implements `raft.Applier`.
- Operations: `UpdateRange`, `GetShard(key)`.

#### [MODIFY] [cmd/meta-node/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/meta-node/main.go)
- Initialize `RaftNode` with `MetaStore`.
- Expose `/route?key=` endpoint.

#### [NEW] [internal/router/router.go](file:///c:/PROJECTS/GOD/ShardDB/internal/router/router.go)
- `Router` struct in Proxy.
- Fetches/Caches RangeMap from Meta-Node.
- `Route(key)` returns Shard Address.

#### [MODIFY] [cmd/proxy/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/proxy/main.go)
- Use `Router` to forward SQL requests to appropriate shards.

### Verification
- **Test:** `TestRangeMap` (unit).
- **Integration:** Start Meta + Shards + Proxy. Client -> Proxy -> Shard.

## Phase 4: Cross-shard Transactions (2PC)
**Goal:** Implement Atomic Transactions across shards using Two-Phase Commit (2PC).

### Proposed Changes
- [x] [Modify] `internal/txn/coordinator.go`: Check `shardWrites` count. If 1, use 1PC path.
- [x] [Modify] `cmd/shard-node/main.go`: Add `/txn/oneshot` endpoint.
- [x] [Modify] `Applier` in `shard-node`: Handle `ONESHOT` command (Prepare + Commit).
#### [MODIFY] [internal/txn/coordinator.go](file:///c:/PROJECTS/GOD/ShardDB/internal/txn/coordinator.go)
- `Coordinator` managing transaction state (Active, Preparing, Committed, Aborted).
- Assigns `TxnID` (Global Timestamp from Meta ideally, or unique UUID/HLC).

## Phase 5: Query Planner & Distributed Execution

## Goal
Enable the Proxy to intelligently route queries based on the `WHERE` clause. Support "Scatter-Gather" for queries that span multiple shards (e.g., `SELECT * FROM table`).

## User Review Required
> [!NOTE]
> The Planner will be a Rule-Based Optimizer (RBO) MVP. It will heavily rely on the presence of the Sharding Key in the `WHERE` clause.
> - **Point Query**: `WHERE id = X` -> Routes to Single Shard.
> - **Range Query**: `WHERE id > X` -> Routes to relevant Shards (Requires RangeMap updates).
> - **Scatter Query**: `SELECT *` (No filter) -> Broadcasts to ALL Shards and merges results.

## Proposed Changes

### [internal/planner]
#### [NEW] [planner.go](file:///c:/PROJECTS/GOD/ShardDB/internal/planner/planner.go)
- Define `Plan` struct.
- `Analyze(sql)`: Returns `PlanType` (Single, Multi, Broadcast) and `TargetShards`.
- Extracts Logic:
    - Parse SQL.
    - Walk `WHERE` clause.
    - If `RoutingKey` == Constant, resolve Shard -> Single.
    - Else -> Broadcast.

### [cmd/proxy/main.go]
#### [MODIFY] [main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/proxy/main.go)
- Update `/execute` handler.
- Use `planner.Analyze`.
- If `Single`: Forward as before.
- If `Broadcast`:
    - Launch Goroutines for each Shard.
    - Send Request.
    - Collect Results (JSON arrays).
    - Merge JSON arrays.
    - Return combined result.

## Verification Plan
### Automated Tests
- `verify_scatter_gather.go`:
    - Insert Data into Shard 1 (`keyA`) and Shard 2 (`keyB`).
    - Query `SELECT * FROM t WHERE key = 'keyA'` -> Returns 1 row (Direct).
    - Query `SELECT * FROM t` -> Returns 2 rows (Merged from S1 and S2).

# Phase 6: Rebalancing & Compaction

## Goal
Implement maintenance tasks to keep the system healthy and scalable.
1. **Version Garbage Collection (GC)**: Prune old MVCC versions to reclaim memory.
2. **Range Splitting**: Allow admins to split hot ranges (Metadata update).

## User Review Required
> [!NOTE]
> **Shard Migration** (moving data between shards) is excluded from this pass to keep scope manageable. We will focus on **Metadata Splitting** (assigning new ranges) and **Local GC**. Data migration would require a distinct "Rebalance" process not yet designed.

## Proposed Changes

### [internal/storage]
#### [MODIFY] [memory.go](file:///c:/PROJECTS/GOD/ShardDB/internal/storage/memory.go)
- Implement `RunGC(safeTs uint64)`.
- Scan BTree, identify keys with `Timestamp < safeTs`.
- Keep only the latest version < safeTs (as the "snapshot" at distinct point). Remove older.
- Or simpler: Remove ALL versions < safeTs except the newest one valid at safeTs?
- **Simplified Policy**: Keep N versions or Time-to-Live (TTL).
- Let's go with **Safe Point GC**: Pass a `minTS`. Delete everything older than `minTS` *if* a newer version exists that is also older than `minTS` (i.e. redundant history).

### [cmd/shard-node]
#### [MODIFY] [main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- Add Background Routine `startGC()`.
- Trigger `engine.RunGC` every 1 minute.

### [internal/meta]
#### [MODIFY] [store.go](file:///c:/PROJECTS/GOD/ShardDB/internal/meta/store.go)
- Implement `SplitRange(key []byte)`.
- Finds the range containing `key`.
- Splits it into `[Start, key)` and `[key, End)`.
- Assigns new ShardIDs (Round-robin or manual).

### [internal/storage]
#### [MODIFY] [memory.go](file:///c:/PROJECTS/GOD/ShardDB/internal/storage/memory.go)
- Implement `ScanRange(start, end)` -> returns key/values (Snapshot of range).
- Implement `Ingest(kvs map[string][]byte)` -> Writes multiple keys (Blind put).

### [cmd/shard-node]
#### [MODIFY] [main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- `GET /debug/export?start=...&end=...`: Exports data in range.
- `POST /debug/ingest`: Accepts JSON data and writes to engine.

### [cmd/tools]
#### [NEW] [verify_migration.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/tools/verify_migration/main.go)
- Insert `keyM` into Shard 1.
- Call Shard 1 `/debug/export` for `keyM`.
- Call Shard 2 `/debug/ingest` with data.
- Update Meta: Set Range `[keyM, keyM+1)` -> Shard 2. (Simulated `UPDATE_RANGE`).
- Verify `Proxy` routes `keyM` to Shard 2 (and reads data).

# Phase 7: Observability & Benchmarking

## Goal
Make the system observable and measurable.
1.  **Metrics**: Expose internal counters (RPS, Latency, Storage size) via HTTP.
2.  **Benchmarking**: Stress test the system to establish baseline performance.

## Proposed Changes

### [internal/metrics]
#### [NEW] [metrics.go](file:///c:/PROJECTS/GOD/ShardDB/internal/metrics/metrics.go)
- Simple thread-safe Counters and Gauges.
- `Inc(name string)`, `Set(name string, val int64)`.
- HTTP Handler helper to dump metrics as JSON.

### [cmd/shard-node]
#### [MODIFY] [main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- Instrument `Apply`: Count `trans_applied`, `ops_insert`, `ops_select`.
- Instrument `Raft`: Count `proposals`.
- Expose `GET /metrics`.

### [cmd/proxy]
#### [MODIFY] [main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/proxy/main.go)
- Instrument `Handler`: Count `requests_total`, `requests_error`.
- Measure Latency? (Average over last minute).
- Expose `GET /metrics`.

### [cmd/tools]
#### [NEW] [benchmark/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/tools/benchmark/main.go)
- Flag `-concurrency`: Number of parallel clients.
- Flag `-duration`: Time to run.
- Workload: Mixed Insert/Select.
- Output: Total Requests, RPS, Avg Latency.

## Verification Plan
### Automated Tests
- `benchmark`: Run and observe > 0 RPS.
- `curl /metrics`: Verify counters increase.

#### [MODIFY] [cmd/shard-node/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/shard-node/main.go)
- Add RPCs: `/prepare`, `/commit`, `/abort`.
- `Prepare`: Locks keys (MVCC Pending writes).
- `Commit`: Finalizes writes (Visible At Timestamp).

#### [MODIFY] [internal/storage/engine.go](file:///c:/PROJECTS/GOD/ShardDB/internal/storage/engine.go)
- Extend Engine to support `Pending` writes (Intents).
- `Get` check intents: if intent exists -> Wait/Abort (Optimistic or 2PL).

#### [MODIFY] [cmd/proxy/main.go](file:///c:/PROJECTS/GOD/ShardDB/cmd/proxy/main.go)
- Handle `BEGIN`, `COMMIT`, `ROLLBACK`.
- Proxy acts as Txn Coordinator for Client session.

### Verification
- Integration Test:
    - Txn 1: Insert to Shard 1, Insert to Shard 2. Commit.
    - Txn 2: Read Shard 1, Read Shard 2. Should see both.
    - Failure Case: Fail Shard 2 prepare -> Txn 1 Aborts. Shard 1 Rollback.





