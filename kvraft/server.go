package raftkv

import (
	"encoding/gob"
	"labrpc"
	"log"
	"raft"
	"sync"
	"time"
)

const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

type Op struct {
	Key       string
	Value     string
	Op        string
	ClientId  int64
	RequestId int
}

type RaftKV struct {
	mu             sync.Mutex
	me             int
	rf             *raft.Raft
	applyCh        chan raft.ApplyMsg
	maxraftstate   int

	kvStore        map[string]string
	clientRequests map[int64]int
	notifyChans    map[int]chan OpResult
}

type OpResult struct {
	Err   Err
	Value string
}

func (kv *RaftKV) startOperation(op Op, reply interface{}) bool {
	index, _, isLeader := kv.rf.Start(op)
	if !isLeader {
		return false
	}

	kv.mu.Lock()
	ch := kv.getNotifyChan(index)
	kv.mu.Unlock()

	select {
	case result := <-ch:
		switch rep := reply.(type) {
		case *GetReply:
			rep.Err = result.Err
			rep.Value = result.Value
		case *PutAppendReply:
			rep.Err = result.Err
		}
	case <-time.After(500 * time.Millisecond):
		return false
	}

	kv.mu.Lock()
	kv.deleteNotifyChan(index)
	kv.mu.Unlock()
	return true
}

func (kv *RaftKV) Get(args *GetArgs, reply *GetReply) {
	op := Op{
		Key:       args.Key,
		Op:        "Get",
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
	}
	if !kv.startOperation(op, reply) {
		reply.WrongLeader = true
	}
}

func (kv *RaftKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	op := Op{
		Key:       args.Key,
		Value:     args.Value,
		Op:        args.Op,
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
	}
	if !kv.startOperation(op, reply) {
		reply.WrongLeader = true
	}
}

func (kv *RaftKV) applyLoop() {
	for msg := range kv.applyCh {
		if msg.UseSnapshot {
			continue
		}

		op := msg.Command.(Op)
		kv.mu.Lock()

		if lastReq, ok := kv.clientRequests[op.ClientId]; !ok || op.RequestId > lastReq {
			switch op.Op {
			case "Put":
				kv.kvStore[op.Key] = op.Value
			case "Append":
				kv.kvStore[op.Key] += op.Value
			}
			kv.clientRequests[op.ClientId] = op.RequestId
		}

		var result OpResult
		if op.Op == "Get" {
			if value, exists := kv.kvStore[op.Key]; exists {
				result = OpResult{Err: OK, Value: value}
			} else {
				result = OpResult{Err: ErrNoKey}
			}
		} else {
			result = OpResult{Err: OK}
		}

		if ch, ok := kv.notifyChans[msg.Index]; ok {
			ch <- result
		}
		kv.mu.Unlock()
	}
}

func (kv *RaftKV) getNotifyChan(index int) chan OpResult {
	if _, ok := kv.notifyChans[index]; !ok {
		kv.notifyChans[index] = make(chan OpResult, 1)
	}
	return kv.notifyChans[index]
}

func (kv *RaftKV) deleteNotifyChan(index int) {
	delete(kv.notifyChans, index)
}

func (kv *RaftKV) Kill() {
	kv.rf.Kill()
}

func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *RaftKV {
	gob.Register(Op{})

	kv := &RaftKV{
		me:             me,
		maxraftstate:   maxraftstate,
		applyCh:        make(chan raft.ApplyMsg),
		kvStore:        make(map[string]string),
		clientRequests: make(map[int64]int),
		notifyChans:    make(map[int]chan OpResult),
	}

	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	go kv.applyLoop()

	return kv
}
