package raftkv

import "labrpc"
import "crypto/rand"
import "math/big"

type Clerk struct {
	servers    []*labrpc.ClientEnd
	clientId   int64
	requestId  int
	lastLeader int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.clientId = nrand()
	ck.requestId = 0
	ck.lastLeader = 0
	return ck
}

func (ck *Clerk) Get(key string) string {
	ck.requestId++
	args := GetArgs{
		Key:       key,
		ClientId:  ck.clientId,
		RequestId: ck.requestId,
	}

	for {
		server := ck.lastLeader
		var reply GetReply
		ok := ck.servers[server].Call("RaftKV.Get", &args, &reply)
		if ok && !reply.WrongLeader {
			if reply.Err == OK {
				return reply.Value
			} else if reply.Err == ErrNoKey {
				return ""
			}
		}
		ck.lastLeader = (ck.lastLeader + 1) % len(ck.servers)
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}

func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}

func (ck *Clerk) PutAppend(key string, value string, op string) {
	ck.requestId++
	args := PutAppendArgs{
		Key:       key,
		Value:     value,
		Op:        op,
		ClientId:  ck.clientId,
		RequestId: ck.requestId,
	}

	for {
		server := ck.lastLeader
		var reply PutAppendReply
		ok := ck.servers[server].Call("RaftKV.PutAppend", &args, &reply)
		if ok && !reply.WrongLeader && reply.Err == OK {
			return
		}
		ck.lastLeader = (ck.lastLeader + 1) % len(ck.servers)
	}
}
