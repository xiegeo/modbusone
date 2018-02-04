package modbusone

import (
	"fmt"
	"io"
	"testing"
)

type reader func(p []byte) (n int, err error)

func (r reader) Read(p []byte) (n int, err error) {
	return r(p)
}

type writer func(p []byte) (n int, err error)

func (w writer) Write(p []byte) (n int, err error) {
	return w(p)
}

var expectA = true //expect A to talk

func connectToMockServers(slaveID byte) (*RTUClient, *SimpleHandler, *SimpleHandler) {

	//pipe from client to 2 servers
	ra, w1a := io.Pipe()
	rb, w1b := io.Pipe()
	w1 := io.MultiWriter(w1a, w1b)

	//pipe from 2 servers to client
	r2, w2 := io.Pipe()
	wa := writer(func(p []byte) (n int, err error) {
		if !expectA {
			panic("expectA is false when A talked")
		}
		return w2.Write(p)
	})
	wb := writer(func(p []byte) (n int, err error) {
		if expectA {
			panic("expectA is true when B talked")
		}
		return w2.Write(p)
	})

	cc := newMockSerial(r2, w1)                              //client connection
	sa := newMockSerial(ra, wa)                              //server a connection
	sb := NewFailbackConn(newMockSerial(rb, wb), true, true) //server b connection

	serverA := NewRTUServer(sa, slaveID)
	serverB := NewRTUServer(sb, slaveID)
	client := NewRTUCLient(cc, slaveID)

	var holdingRegistersA [100]uint16
	var holdingRegistersB [100]uint16
	var holdingRegistersC [100]uint16
	shA := &SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("Read shA from %v, quantity %v\n", address, quantity)
			return holdingRegistersA[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("Write shA %v, quantity %v\n", address, len(values))
			for i, v := range values {
				holdingRegistersA[address+uint16(i)] = v
			}
			return nil
		},
	}
	shB := &SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("Read shB from %v, quantity %v\n", address, quantity)
			return holdingRegistersB[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("Write shB from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				holdingRegistersB[address+uint16(i)] = v
			}
			return nil
		},
	}
	shC := &SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("Read client from %v, quantity %v\n", address, quantity)
			return holdingRegistersC[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("Write client from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				holdingRegistersC[address+uint16(i)] = v
			}
			return nil
		},
	}
	go serverA.Serve(shA)
	go serverB.Serve(shB)
	go client.Serve(shC)
	return client, shA, shB
}

func TestFailback(t *testing.T) {
	id := byte(3)
	client, _, _ := connectToMockServers(id)
	type tc struct {
		fc   FunctionCode
		size uint16
	}
	testCases := []tc{
		{FcWriteSingleRegister, 1},
		{FcWriteSingleRegister, 2},
	}
	for i, ts := range testCases {
		reqs, err := MakePDURequestHeadersSized(ts.fc, 0, ts.size, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = DoTransactions(client, id, reqs)
		if err != nil {
			t.Fatal(err)
		}
	}

}
