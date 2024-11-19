package raft

import (
    "math/rand"
    "sync"
    "time"
    "labrpc"
    "bytes"
    "encoding/gob"
    "sync/atomic"
)

type Raft struct {
    mu        sync.Mutex
    peers     []*labrpc.ClientEnd
    persister *Persister
    me        int 

    currentTerm int
    votedFor    int
    log         []LogEntry

    commitIndex int
    lastApplied int

    nextIndex  []int
    matchIndex []int

    state              NodeState
    electionResetEvent time.Time
    leaderId           int
    applyCh            chan ApplyMsg
    applyCond          *sync.Cond
    dead               int32
}

type NodeState int

type LogEntry struct {
    Term    int
    Command interface{}
}

type ApplyMsg struct {
    Command      interface{}
    UseSnapshot  bool
    Index        int
}

const (
    Follower NodeState = iota
    Candidate
    Leader
)

type RequestVoteReply struct {
    Term        int
    VoteGranted bool
}

type RequestVoteArgs struct {
    Term         int
    CandidateId  int
    LastLogIndex int
    LastLogTerm  int
}

type AppendEntriesReply struct {
    Term    int
    Success bool
}

type AppendEntriesArgs struct {
    Term         int
    LeaderId     int
    PrevLogIndex int
    PrevLogTerm  int
    Entries      []LogEntry
    LeaderCommit int
}

func (rf *Raft) GetState() (int, bool) {
    rf.mu.Lock()
    defer rf.mu.Unlock()
    return rf.currentTerm, rf.state == Leader
}

func (rf *Raft) Start(command interface{}) (int, int, bool) {
    rf.mu.Lock()
    defer rf.mu.Unlock()

    if rf.state != Leader {
        return -1, rf.currentTerm, false
    }

    index := len(rf.log)
    term := rf.currentTerm
    rf.log = append(rf.log, LogEntry{Term: term, Command: command})
    rf.persist()

    return index, term, true
}

func (rf *Raft) Kill() {
    atomic.StoreInt32(&rf.dead, 1)
}

func Make(peers []*labrpc.ClientEnd, me int, persister *Persister, applyCh chan ApplyMsg) *Raft {
    rf := &Raft{}
    rf.peers = peers
    rf.persister = persister
    rf.me = me
    rf.applyCh = applyCh

    rf.currentTerm = 0
    rf.votedFor = -1
    rf.log = make([]LogEntry, 0)
    rf.commitIndex = 0
    rf.lastApplied = 0
    rf.nextIndex = make([]int, len(peers))
    rf.matchIndex = make([]int, len(peers))
    rf.state = Follower
    rf.electionResetEvent = time.Now()
    rf.applyCond = sync.NewCond(&rf.mu)

    rf.readPersist(persister.ReadRaftState())

    go rf.electionTimer()
    go rf.applyCommitted()

    return rf
}

func (rf *Raft) electionTimer() {
    for !rf.killed() {
        time.Sleep(10 * time.Millisecond)

        rf.mu.Lock()
        if rf.state == Leader {
            rf.mu.Unlock()
            continue
        }

        if time.Since(rf.electionResetEvent) > rf.electionTimeout() {
            rf.startElection()
        }
        rf.mu.Unlock()
    }
}

func (rf *Raft) electionTimeout() time.Duration {
    return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (rf *Raft) startElection() {
    rf.state = Candidate
    rf.currentTerm++
    rf.votedFor = rf.me
    rf.persist()
    rf.electionResetEvent = time.Now()

    args := RequestVoteArgs{
        Term:         rf.currentTerm,
        CandidateId:  rf.me,
        LastLogIndex: len(rf.log) - 1,
        LastLogTerm:  rf.lastLogTerm(),
    }

    var votes int32 = 1
    for i := range rf.peers {
        if i != rf.me {
            go rf.requestVote(i, args, &votes)
        }
    }
}

func (rf *Raft) requestVote(server int, args RequestVoteArgs, votes *int32) {
    var reply RequestVoteReply
    if rf.sendRequestVote(server, args, &reply) {
        rf.mu.Lock()
        defer rf.mu.Unlock()

        if rf.state != Candidate || rf.currentTerm != args.Term {
            return
        }

        if reply.Term > rf.currentTerm {
            rf.becomeFollower(reply.Term)
            return
        }

        if reply.VoteGranted {
            atomic.AddInt32(votes, 1)
            if atomic.LoadInt32(votes) > int32(len(rf.peers)/2) {
                rf.becomeLeader()
            }
        }
    }
}

func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
    rf.mu.Lock()
    defer rf.mu.Unlock()

    if args.Term < rf.currentTerm {
        reply.Term = rf.currentTerm
        reply.VoteGranted = false
        return
    }

    if args.Term > rf.currentTerm {
        rf.becomeFollower(args.Term)
    }

    if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) &&
        rf.isLogUpToDate(args.LastLogIndex, args.LastLogTerm) {
        rf.votedFor = args.CandidateId
        rf.persist()
        reply.VoteGranted = true
        rf.electionResetEvent = time.Now()
    } else {
        reply.VoteGranted = false
    }

    reply.Term = rf.currentTerm
    rf.persist()
}

func (rf *Raft) isLogUpToDate(lastLogIndex, lastLogTerm int) bool {
    if lastLogIndex == -1 {
        return true
    }
    lastIndex := len(rf.log) - 1
    return lastLogTerm > rf.log[lastIndex].Term ||
        (lastLogTerm == rf.log[lastIndex].Term && lastLogIndex >= lastIndex)
}

func (rf *Raft) becomeFollower(term int) {
    rf.state = Follower
    rf.currentTerm = term
    rf.votedFor = -1
    rf.electionResetEvent = time.Now()
    rf.persist()
}

func (rf *Raft) becomeLeader() {
    rf.state = Leader
    rf.leaderId = rf.me
    rf.persist()
    for i := range rf.nextIndex {
        rf.nextIndex[i] = len(rf.log)
        rf.matchIndex[i] = 0
    }
    go rf.heartbeat()
}

func (rf *Raft) heartbeat() {
    for !rf.killed() {
        rf.mu.Lock()
        if rf.state != Leader {
            rf.mu.Unlock()
            return
        }
        rf.mu.Unlock()

        for i := range rf.peers {
            if i != rf.me {
                go rf.sendAppendEntries(i)
            }
        }

        time.Sleep(100 * time.Millisecond)
    }
}

func (rf *Raft) sendAppendEntries(server int) {
    rf.mu.Lock()
    if rf.state != Leader {
        rf.mu.Unlock()
        return
    }

    prevLogIndex := rf.nextIndex[server] - 1
    prevLogTerm := 0
    if prevLogIndex >= 0 {
        prevLogTerm = rf.log[prevLogIndex].Term
    }

    entries := append([]LogEntry(nil), rf.log[rf.nextIndex[server]:]...)

    args := AppendEntriesArgs{
        Term:         rf.currentTerm,
        LeaderId:     rf.me,
        PrevLogIndex: prevLogIndex,
        PrevLogTerm:  prevLogTerm,
        Entries:      entries,
        LeaderCommit: rf.commitIndex,
    }
    rf.mu.Unlock()

    var reply AppendEntriesReply
    if rf.sendAppendEntriesRPC(server, &args, &reply) {
        rf.mu.Lock()
        defer rf.mu.Unlock()

        if reply.Term > rf.currentTerm {
            rf.becomeFollower(reply.Term)
            return
        }

        if reply.Success {
            rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
            rf.matchIndex[server] = rf.nextIndex[server] - 1
            rf.updateCommitIndex()
        } else {
            rf.nextIndex[server] = max(1, rf.nextIndex[server]-1)
        }
    }
}

func (rf *Raft) updateCommitIndex() {
    for N := len(rf.log) - 1; N > rf.commitIndex; N-- {
        count := 1
        for i := range rf.peers {
            if i != rf.me && rf.matchIndex[i] >= N && rf.log[N].Term == rf.currentTerm {
                count++
            }
        }
        if count > len(rf.peers)/2 {
            rf.commitIndex = N
            rf.applyCond.Broadcast()
            break
        }
    }
}

func (rf *Raft) applyCommitted() {
    for {
        rf.mu.Lock()
        for rf.lastApplied < rf.commitIndex {
            rf.lastApplied++
            msg := ApplyMsg{
                Command:      rf.log[rf.lastApplied].Command,
                Index:        rf.lastApplied,
            }
            rf.mu.Unlock()
            rf.applyCh <- msg
            rf.mu.Lock()
        }
        rf.applyCond.Wait()
        rf.mu.Unlock()
    }
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
    rf.mu.Lock()
    defer rf.mu.Unlock()

    if args.Term < rf.currentTerm {
        reply.Term = rf.currentTerm
        reply.Success = false
        return
    }

    if args.Term > rf.currentTerm {
        rf.becomeFollower(args.Term)
    }
    rf.electionResetEvent = time.Now()
    rf.leaderId = args.LeaderId

    if args.PrevLogIndex >= len(rf.log) ||
        (args.PrevLogIndex >= 0 && rf.log[args.PrevLogIndex].Term != args.PrevLogTerm) {
        reply.Term = rf.currentTerm
        reply.Success = false
        return
    }

    index := args.PrevLogIndex + 1
    for i, entry := range args.Entries {
        if index < len(rf.log) {
            if rf.log[index].Term != entry.Term && i >= 0 {
                rf.log = rf.log[:index]
                rf.persist()
                break
            }
        }
        index++
    }

    if index <= len(rf.log) || len(args.Entries) > 0 {
        rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
        rf.persist()
    }

    if args.LeaderCommit > rf.commitIndex {
        rf.commitIndex = min(args.LeaderCommit, len(rf.log)-1)
        rf.applyCond.Broadcast()
    }

    reply.Term = rf.currentTerm
    reply.Success = true
}

func (rf *Raft) sendAppendEntriesRPC(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
    ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
    return ok
}

func (rf *Raft) persist() {
    w := new(bytes.Buffer)
    e := gob.NewEncoder(w)
    e.Encode(rf.currentTerm)
    e.Encode(rf.votedFor)
    e.Encode(rf.log)
    data := w.Bytes()
    rf.persister.SaveRaftState(data)
}

func (rf *Raft) readPersist(data []byte) {
    if data == nil || len(data) == 0 {
        return
    }
    r := bytes.NewBuffer(data)
    d := gob.NewDecoder(r)
    d.Decode(&rf.currentTerm)
    d.Decode(&rf.votedFor)
    d.Decode(&rf.log)
}

func (rf *Raft) lastLogTerm() int {
    if len(rf.log) > 0 {
        return rf.log[len(rf.log)-1].Term
    }
    return 0
}

func (rf *Raft) killed() bool {
    return atomic.LoadInt32(&rf.dead) == 1
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
    ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
    return ok
}