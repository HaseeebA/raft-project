package raftkv

const (
	OK       = "OK"
	ErrNoKey = "ErrNoKey"
)

type Err string

type PutAppendArgs struct {
	Key       string
	Value     string
	Op        string 
	ClientId  int64
	RequestId int
}

type PutAppendReply struct {
	WrongLeader bool
	Err         Err
}

type GetArgs struct {
	Key       string
	ClientId  int64
	RequestId int
}

type GetReply struct {
	WrongLeader bool
	Err         Err
	Value       string
}
