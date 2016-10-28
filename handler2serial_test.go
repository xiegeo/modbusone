package modbusone

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

type mockSerial struct {
	io.Reader
	io.Writer
	LastWritten []byte
}

func newMockSerial(r io.Reader, w io.Writer) *mockSerial {
	return &mockSerial{Reader: r, Writer: w}
}
func (s *mockSerial) Write(data []byte) (n int, err error) {
	s.LastWritten = data
	return s.Writer.Write(data)
}
func (s *mockSerial) Close() error                   { return nil }
func (s *mockSerial) MinDelay() time.Duration        { return 0 }
func (s *mockSerial) BytesDelay(n int) time.Duration { return 0 }

func TestHandler(t *testing.T) {
	DebugOut = os.Stdout
	slaveId := byte(0x11)
	r1, w1 := io.Pipe() //pipe from client to server
	r2, w2 := io.Pipe() //pipe from server to client

	cc := newMockSerial(r2, w1) //client connection
	sc := newMockSerial(r1, w2) //server connection

	client := NewRTUCLient(cc, slaveId)
	server := NewRTUServer(sc, slaveId)

	subtest := t

	ch := &SimpleHandler{
		OnErrorImp: func(req PDU, errRep PDU) {
			subtest.Errorf("client handler recived error:%v in request:%v", errRep, req)
		},
	}
	sh := &SimpleHandler{
		OnErrorImp: func(req PDU, errRep PDU) {
			subtest.Errorf("server handler recived error:%v in request:%v", errRep, req)
		},
	}

	go client.Serve(ch)
	go server.Serve(sh)

	t.Run(fmt.Sprintf("Read Coil Status (FC=01)"), func(t *testing.T) {
		subtest = t
		request := RTU([]byte{0x11, 0x01, 0x00, 0x13, 0x00, 0x25, 0x0E, 0x84})
		response := RTU([]byte{0x11, 0x01, 0x05, 0xCD, 0x6B, 0xB2, 0x0E, 0x1B, 0x45, 0xE6})
		vs := []bool{
			true, false, true, true, false, false, true, true,
			true, true, false, true, false, true, true, false,
			false, true, false, false, true, true, false, true,
			false, true, true, true, false, false, false, false,
			true, true, false, true, true, false, false, false}
		sh.ReadCoils = func(address, quantity uint16) ([]bool, error) {
			if address != 0x13 {
				t.Errorf("server unexpected address %x", address)
			}
			if quantity != 0x25 {
				t.Errorf("server unexpected quantity %x", address)
			}
			return vs, nil
		}
		ch.WriteCoils = func(address uint16, values []bool) error {
			if address != 0x13 {
				t.Errorf("client unexpected address %x", address)
			}
			if len(values) != 0x25 {
				t.Errorf("client unexpected quantity %x", address)
			}
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value flipped", i)
				}
			}
			return nil
		}
		err := <-client.StartTransaction(request.fastGetPDU())
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(cc.LastWritten, request) {
			t.Fatal("request is not as expected")
		}
		if !bytes.Equal(sc.LastWritten, response) {
			t.Fatal("response is not as expected")
		}
	})

}
