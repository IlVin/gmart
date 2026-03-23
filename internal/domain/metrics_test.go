package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpType_String(t *testing.T) {
	tests := []struct {
		name string
		op   OpType
		want string
	}{
		{
			name: "query operation",
			op:   OpQuery,
			want: "query",
		},
		{
			name: "exec operation",
			op:   OpExec,
			want: "exec",
		},
		{
			name: "transaction operation",
			op:   OpTx,
			want: "tx",
		},
		{
			name: "pool operation",
			op:   OpPool,
			want: "pool",
		},
		{
			name: "bcrypt operation",
			op:   OpBcrypt,
			want: "bcrypt",
		},
		{
			name: "custom or unknown operation",
			op:   OpType("custom"),
			want: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем метод String()
			assert.Equal(t, tt.want, tt.op.String())

			// Проверяем явное приведение (используется в мапах или при формировании ключей метрик)
			assert.Equal(t, tt.want, string(tt.op))
		})
	}
}
