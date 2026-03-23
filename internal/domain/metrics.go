package domain

type OpType string

const (
	OpQuery  OpType = "query"
	OpExec   OpType = "exec"
	OpTx     OpType = "tx"
	OpPool   OpType = "pool"
	OpBcrypt OpType = "bcrypt"
)

func (o OpType) String() string {
	return string(o)
}
