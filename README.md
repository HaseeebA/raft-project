# Distributed Consensus with Raft

A robust implementation of the Raft consensus algorithm in Go, providing a foundation for building distributed systems that require strong consistency guarantees. This implementation follows the Raft paper specifications while providing practical abstractions for real-world applications.

## Overview

Raft is a consensus algorithm designed to be more understandable than Paxos while providing the same strong consistency guarantees. This implementation includes all core Raft features:

- Leader Election
- Log Replication
- Safety Guarantees
- Membership Changes
- Log Compaction

The project is structured into multiple packages, each serving a specific purpose in the distributed system architecture.

## Project Structure

```
.
├── raft/           # Core Raft consensus implementation
├── kvraft/         # Example key-value store using Raft
└── labrpc/         # Simulated RPC framework for testing
```

### Core Components

#### 1. Raft Package (`raft/`)
The heart of the consensus implementation, handling:
- State machine transitions (Follower → Candidate → Leader)
- Log management and replication
- Leader election with randomized timeouts
- Persistent state management
- Safety guarantees through term numbers and log matching

Key files:
- `raft.go`: Core Raft state machine implementation
- `persist.go`: Persistence mechanisms for Raft state
- `config.go`: Configuration and setup utilities

#### 2. Key-Value Store (`kvraft/`)
A practical example of building a distributed service using Raft:
- Replicated key-value storage
- Client request handling
- State machine replication
- Linearizable operations (Get/Put/Append)

#### 3. RPC Framework (`labrpc/`)
A testing-focused RPC implementation that simulates:
- Network delays
- Packet loss
- Network partitions
- Message reordering

## Implementation Details

### Raft State Machine

Each Raft node maintains the following state:

**Persistent State:**
- `currentTerm`: Latest term server has seen
- `votedFor`: CandidateId that received vote in current term
- `log[]`: Log entries containing commands and terms

**Volatile State:**
- `commitIndex`: Index of highest log entry known to be committed
- `lastApplied`: Index of highest log entry applied to state machine
- `state`: Current state (FOLLOWER, CANDIDATE, or LEADER)

### Leader Election

The leader election process follows these steps:

1. **Timeout Initialization**
   - Followers maintain randomized election timeouts
   - No heartbeat received → transition to Candidate state

2. **Voting Process**
   - Candidate increments term and requests votes
   - Votes granted based on log completeness
   - Winner needs majority of votes

3. **Leadership Establishment**
   - New leader begins sending heartbeats
   - Establishes authority for the term
   - Manages log replication

### Log Replication

The log replication mechanism ensures consistency:

1. **Client Requests**
   - Clients send commands to the leader
   - Leader appends to local log
   - Initiates replication to followers

2. **Consistency Checking**
   - AppendEntries RPCs include previous log terms
   - Followers verify log matching
   - Leader tracks follower progress

3. **Commitment**
   - Entry committed when replicated to majority
   - Leader notifies followers of commits
   - State machine applies committed entries

## Usage Guide

### Basic Setup

```go
// Initialize a new Raft server
rf := raft.Make(peers, me, persister, applyCh)

// Create a key-value store client
kv := kvraft.NewClient(servers)

// Example operations
kv.Put("key1", "value1")
value := kv.Get("key1")
kv.Append("key1", "more-data")
```

### Configuration Options

Raft servers can be configured with various parameters:

```go
type Config struct {
    ElectionTimeout  time.Duration
    HeartbeatPeriod time.Duration
    MaxLogSize      int
    SnapshotInterval int
}
```

## Testing

The implementation includes comprehensive tests:

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run specific test suite
go test ./raft -run TestInitialElection
```

### Test Scenarios Cover:

- Leader Election
  - Initial election
  - Re-election on leader failure
  - Split vote scenarios

- Log Replication
  - Basic log replication
  - Conflict resolution
  - Log consistency checks

- Fault Tolerance
  - Network partitions
  - Node failures
  - Message losses
  - Split brain prevention

## Performance Considerations

The implementation includes several optimizations:

1. **Batching**
   - Combines multiple log entries in single AppendEntries
   - Reduces network overhead
   - Improves throughput

2. **Log Compaction**
   - Periodic snapshotting
   - Reduces memory usage
   - Speeds up restart times

3. **Concurrent Processing**
   - Parallel request handling
   - Non-blocking operations where possible
   - Efficient goroutine management

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch
3. Implement changes with tests
4. Submit a pull request

## Further Reading

- [Raft Paper](https://raft.github.io/raft.pdf)
- [Raft Visualization](https://raft.github.io/)
- [Implementation Notes](./docs/IMPLEMENTATION.md)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.