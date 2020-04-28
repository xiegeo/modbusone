package modbusone

import (
	"bytes"
	"io"
	"testing"
)

func Test_readTCP(t *testing.T) {
	type args struct {
		r  io.Reader
		bs []byte
	}
	tests := []struct {
		name    string
		args    args
		wantN   int
		wantErr bool
	}{
		{
			name: "sample",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}),
				bs: make([]byte, MaxPDUSize),
			},
			wantN:   12,
			wantErr: false,
		}, {
			name: "early EOF",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x11, 0x03, 0x00, 0x6B, 0x00}),
				bs: make([]byte, MaxPDUSize),
			},
			wantErr: true,
		}, {
			name: "length too long A",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}),
				bs: make([]byte, MaxPDUSize),
			},
			wantErr: true,
		}, {
			name: "length too long B",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0xff, 0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}),
				bs: make([]byte, MaxPDUSize),
			},
			wantErr: true,
		}, {
			name: "unexpected Protocol Identifier A",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x01, 0x00, 0x00, 0x06, 0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}),
				bs: make([]byte, MaxPDUSize),
			},
			wantErr: true,
		}, {
			name: "unexpected Protocol Identifier B",
			args: args{
				r:  bytes.NewBuffer([]byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x06, 0x11, 0x03, 0x00, 0x6B, 0x00, 0x03}),
				bs: make([]byte, MaxPDUSize),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotN, err := readTCP(tt.args.r, tt.args.bs)
			if !tt.wantErr && gotN != tt.wantN {
				t.Errorf("readTCP() = %v, want %v", gotN, tt.wantN)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("readTCP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
