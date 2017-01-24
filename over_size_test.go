package modbusone

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

var _ = os.Stdout

func connectToMockServer(slaveID byte) io.ReadWriter {

	r1, w1 := io.Pipe() //pipe from client to server
	r2, w2 := io.Pipe() //pipe from server to client

	cc := newMockSerial(r2, w1) //client connection
	sc := newMockSerial(r1, w2) //server connection

	server := NewRTUServer(sc, slaveID)

	sh := &SimpleHandler{
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			return nil
		},
	}
	go server.Serve(sh)
	return cc
}

func TestOverSize(t *testing.T) {
	//DebugOut = os.Stdout
	slaveID := byte(0x11)
	cc := connectToMockServer(slaveID)
	pdu := PDU(
		append([]byte{byte(FcWriteMultipleRegisters),
			0, 0, 0, 200, 0}, make([]byte, 400)...))
	rtu := MakeRTU(slaveID, pdu)
	cc.Write([]byte(rtu))

	rep := make([]byte, 1000)
	nchan := make(chan int)
	go func() {
		n, _ := cc.Read(rep)
		nchan <- n
	}()
	timeout := time.NewTimer(time.Second / 20)
	var n int
	select {
	case n = <-nchan:
		t.Fatalf("should not complete read %x", rep[:n])
	case <-timeout.C:
	}

	OverSizeSupport = true
	OverSizeMaxRTU = 512
	defer func() {
		OverSizeSupport = false
	}()

	//New server with OverSizeSupport
	cc = connectToMockServer(slaveID)
	cc.Write([]byte(rtu))
	go func() {
		n, _ := cc.Read(rep)
		nchan <- n
	}()
	timeout.Reset(time.Second)
	select {
	case n = <-nchan:
		if "1110000000c8c30f" != fmt.Sprintf("%x", rep[:n]) {
			t.Fatalf("got unexpected read %x", rep[:n])
		}
	case <-timeout.C:
		t.Fatalf("should not time out")
	}
}
