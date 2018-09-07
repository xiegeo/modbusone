package modbusone_test

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/xiegeo/modbusone"
)

var _ = os.Stdout

func connectToMockServer(slaveID byte) io.ReadWriteCloser {

	r1, w1 := io.Pipe() //pipe from client to server
	r2, w2 := io.Pipe() //pipe from server to client

	cc := newMockSerial("c", r2, w1, w1, w2) //client connection
	sc := newMockSerial("s", r1, w2, w2)     //server connection

	server := NewRTUServer(sc, slaveID)

	sh := &SimpleHandler{
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			return nil
		},
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			return make([]uint16, quantity), nil
		},
	}
	go server.Serve(sh)
	return cc
}

func TestOverSize(t *testing.T) {
	//DebugOut = os.Stdout
	slaveID := byte(0x11)
	cct := connectToMockServer(slaveID)
	defer cct.Close()
	pdu := PDU(
		append([]byte{byte(FcWriteMultipleRegisters),
			0, 0, 0, 200, 0}, make([]byte, 400)...))
	rtu := MakeRTU(slaveID, pdu)
	cct.Write([]byte(rtu))

	rep := make([]byte, 1000)
	nchan := make(chan int)
	go func() {
		n, err := cct.Read(rep)
		if n == 0 && err != nil {
			return
		}
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
	cc := connectToMockServer(slaveID)
	defer cc.Close()
	cc.Write([]byte(rtu))
	go func() {
		for {
			n, err := cc.Read(rep)
			if n == 0 && err != nil {
				return
			}
			nchan <- n
		}
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

	pdu = PDU([]byte{byte(FcReadHoldingRegisters),
		0, 0, 0, 200})
	rtu = MakeRTU(slaveID, pdu)
	cc.Write([]byte(rtu))

	select {
	case n = <-nchan:
		//0x90 is from 200 * 2 = 0x0190
		if "1103900000" != fmt.Sprintf("%x", rep[:5]) {
			t.Fatalf("got unexpected read %x", rep[:n])
		}
	case <-timeout.C:
		t.Fatalf("should not time out")
	}
}
