package modbusone

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

var _ = os.Stdin

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

//TestHandler runs through each of simplymodbus.ca's samples, conforms both
//end-to-end behavior and wire format
func TestHandler(t *testing.T) {
	//DebugOut = os.Stdout
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
			subtest.Errorf("client handler recived error:%x in request:%x", errRep, req)
		},
	}
	sh := &SimpleHandler{
		OnErrorImp: func(req PDU, errRep PDU) {
			subtest.Errorf("server handler recived error:%x in request:%x", errRep, req)
		},
	}

	go client.Serve(ch)
	go server.Serve(sh)

	testTrans := func(header PDU, req, res RTU) {
		t := subtest
		err := <-client.StartTransaction(header)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(cc.LastWritten, req) {
			t.Fatal("request is not as expected")
		}
		if !bytes.Equal(sc.LastWritten, res) {
			t.Fatal("response is not as expected")
		}
		sh.ReadCoils = nil
		ch.WriteCoils = nil
		cc.LastWritten = nil
		sc.LastWritten = nil
	}

	t.Run(fmt.Sprintf("Read Coil Status (FC=01)"), func(t *testing.T) {
		subtest = t
		header, err := FcReadCoils.MakeRequestHeader(0x0013, 0x0025)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x01, 0x00, 0x13, 0x00, 0x25, 0x0E, 0x84})
		response := RTU([]byte{0x11, 0x01, 0x05, 0xCD, 0x6B, 0xB2, 0x0E, 0x1B, 0x45, 0xE6})
		vs := []bool{
			true, false, true, true, false, false, true, true,
			true, true, false, true, false, true, true, false,
			false, true, false, false, true, true, false, true,
			false, true, true, true, false, false, false, false,
			true, true, false, true, true}
		sh.ReadCoils = func(address, quantity uint16) ([]bool, error) {
			return vs, nil
		}
		ch.WriteCoils = func(address uint16, values []bool) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Read Input Status (FC=02)"), func(t *testing.T) {
		subtest = t
		header, err := FcReadDiscreteInputs.MakeRequestHeader(0x00C4, 0x0016)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x02, 0x00, 0xC4, 0x00, 0x16, 0xBA, 0xA9})
		response := RTU([]byte{0x11, 0x02, 0x03, 0xAC, 0xDB, 0x35, 0x20, 0x18})
		vs := []bool{
			false, false, true, true, false, true, false, true,
			true, true, false, true, true, false, true, true,
			true, false, true, false, true, true}
		sh.ReadDiscreteInputs = func(address, quantity uint16) ([]bool, error) {
			return vs, nil
		}
		ch.WriteDiscreteInputs = func(address uint16, values []bool) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Read Holding Registers (FC=03)"), func(t *testing.T) {
		subtest = t
		header, err := FcReadHoldingRegisters.MakeRequestHeader(0x006B, 0x0003)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x03, 0x00, 0x6B, 0x00, 0x03, 0x76, 0x87})
		response := RTU([]byte{0x11, 0x03, 0x06, 0xAE, 0x41, 0x56, 0x52, 0x43, 0x40, 0x49, 0xAD})
		vs := []uint16{0xAE41, 0x5652, 0x4340}
		sh.ReadHoldingRegisters = func(address, quantity uint16) ([]uint16, error) {
			return vs, nil
		}
		ch.WriteHoldingRegisters = func(address uint16, values []uint16) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Read Input Registers (FC=04)"), func(t *testing.T) {
		subtest = t
		header, err := FcReadInputRegisters.MakeRequestHeader(0x0008, 0x0001)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x04, 0x00, 0x08, 0x00, 0x01, 0xB2, 0x98})
		response := RTU([]byte{0x11, 0x04, 0x02, 0x00, 0x0A, 0xF8, 0xF4})
		vs := []uint16{0x000A}
		sh.ReadInputRegisters = func(address, quantity uint16) ([]uint16, error) {
			return vs, nil
		}
		ch.WriteInputRegisters = func(address uint16, values []uint16) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Write Single Coil (FC=05)"), func(t *testing.T) {
		subtest = t
		header, err := FcWriteSingleCoil.MakeRequestHeader(0x00AC, 1)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x05, 0x00, 0xAC, 0xFF, 0x00, 0x4E, 0x8B})
		response := request
		vs := []bool{true}
		sh.WriteCoils = func(address uint16, values []bool) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		ch.ReadCoils = func(address, quantity uint16) ([]bool, error) {
			return vs, nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Write Single Register (FC=06)"), func(t *testing.T) {
		subtest = t
		header, err := FcWriteSingleRegister.MakeRequestHeader(0x0001, 1)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x06, 0x00, 0x01, 0x00, 0x03, 0x9A, 0x9B})
		response := request
		vs := []uint16{3}
		sh.WriteHoldingRegisters = func(address uint16, values []uint16) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		ch.ReadHoldingRegisters = func(address, quantity uint16) ([]uint16, error) {
			return vs, nil
		}
		testTrans(header, request, response)
	})

	t.Run(fmt.Sprintf("Write Multiple Coils (FC=15)"), func(t *testing.T) {
		subtest = t
		header, err := FcWriteMultipleCoils.MakeRequestHeader(0x0013, 0x000A)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x0F, 0x00, 0x13, 0x00, 0x0A, 0x02, 0xCD, 0x01, 0xBF, 0x0B})
		response := RTU([]byte{0x11, 0x0F, 0x00, 0x13, 0x00, 0x0A, 0x26, 0x99})
		vs := []bool{
			true, false, true, true, false, false, true, true,
			true, false}
		sh.WriteCoils = func(address uint16, values []bool) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		ch.ReadCoils = func(address, quantity uint16) ([]bool, error) {
			return vs, nil
		}
		testTrans(header, request, response)
	})
	t.Run(fmt.Sprintf("Write Multiple Registers (FC=16)"), func(t *testing.T) {
		subtest = t
		header, err := FcWriteMultipleRegisters.MakeRequestHeader(0x0001, 0x0002)
		if err != nil {
			t.Fatal(err)
		}
		request := RTU([]byte{0x11, 0x10, 0x00, 0x01, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02, 0xC6, 0xF0})
		response := RTU([]byte{0x11, 0x10, 0x00, 0x01, 0x00, 0x02, 0x12, 0x98})
		vs := []uint16{0x000A, 0x0102}
		sh.WriteHoldingRegisters = func(address uint16, values []uint16) error {
			for i, b := range values {
				if vs[i] != b {
					t.Errorf("%v'th value changed", i)
				}
			}
			return nil
		}
		ch.ReadHoldingRegisters = func(address, quantity uint16) ([]uint16, error) {
			return vs, nil
		}
		testTrans(header, request, response)
	})
}
