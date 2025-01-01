# ShardDB Task Checklist (COMPLETE)

## Phase 0: Project Setup
- [x] Initialize Go module and project structure
- [x] Create Docker Compose for multi-node dev environment
- [x] Set up basic logging and configuration

## Phase 1: Single-node Storage & SQL
- [x] Implement in-memory KV storage with WAL (Write Ahead Log)
- [x] Implement MVCC (Multi-Version Concurrency Control) basics
    - [x] Versioned key encoding
    - [x] Transaction start/commit timestamp handling (Basic support in MVCCGet)
- [x] Implement basic SQL Parser (or integrate existing one)
    - [x] Support CREATE TABLE, INSERT, SELECT, UPDATE, DELETE, BEGIN, COMMIT (Partial: Select/Insert basics)
- [x] Implement Simple Local Executor
    - [x] Table Scan
    - [x] Filter, Project
- [x] Unit Tests for Storage and SQL execution

## Phase 2: Raft Integration & Replication
- [x] Integrate `etcd/raft` library
- [x] Implement Raft Node wrapper (start, stop, propose)
- [x] Connect WAL to Raft State Machine (Implemented DiskStorage wrapper)
- [x] Implement Snapshotting (Basic implementation added)
- [x] Verify single-shard replication (3 nodes) (Verified locally)

## Phase 3: Meta-Shard & RangeMap
- [x] Implement Meta-Shard Raft group
- [x] Design and implement RangeMap (Range -> ShardID)
- [x] Implement Proxy/Router basic routing logic
- [x] API for Meta-Shard (GetRange, UpdateRange)

## Phase 4: Distributed Transactions (2PC)
- [x] Implement Shard Node RPCs (`/txn/prepare`, `/txn/commit`, `/txn/abort`).
- [x] Implement `Coordinator` in Proxy.
- [x] Update Storage Engine for Locking & 2PC support.
- [x] Verify End-to-End 2PC Flow.
- [x] Optimization: Single-shard fast path (Implemented OneShot RPC)

## Phase 5: Query Planner & Distributed Execution
- [x] Logical Planner (Simple RBO: Point vs Broadcast)
- [x] Distributed Execution (Scatter-Gather in Proxy)
- [x] Filters push-down (Implemented via PointGet optimization)

## Phase 6: Rebalancing & Compaction
- [x] Range Split logic (Metadata API)
- [x] Shard migration (snapshot transfer) (Implemented Export/Ingest)
- [x] Background compaction (Version GC)
- [x] Version Garbage Collection

## Phase 7: Observability & Benchmarking
- [x] Metrics Package (Zero-dep)
- [x] Instrumentation (Shard Node & Proxy)
- [x] Benchmark Tool

