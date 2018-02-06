package modbusone

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
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

type counter struct {
	reads  int
	writes int
}

func (c *counter) reset() {
	c.reads = 0
	c.writes = 0
}

func newTestHandler(name string) ([]uint16, *SimpleHandler, *counter) {
	var holdingRegisters [100]uint16
	count := counter{}
	shA := &SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("Read %s %v, quantity %v\n", name, address, quantity)
			count.reads += int(quantity)
			return holdingRegisters[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("Write %s %v, quantity %v\n", name, address, len(values))
			count.writes += len(values)
			for i, v := range values {
				holdingRegisters[address+uint16(i)] = v
			}
			return nil
		},
	}
	return holdingRegisters[:], shA, &count
}

func connectToMockServers(slaveID byte) (*RTUClient, *counter, *counter, *counter) {

	//pipes
	ra, wa := io.Pipe() //server a
	rb, wb := io.Pipe() //server b
	rc, wc := io.Pipe() //client

	//everyone writes to everyone else
	wfc := io.MultiWriter(wa, wb)
	wfa := writer(func(p []byte) (n int, err error) {
		if !expectA {
			panic("expectA is false when A talked")
		}
		wb.Write(p)
		return wc.Write(p)
	})
	wfb := writer(func(p []byte) (n int, err error) {
		if expectA {
			panic("expectA is true when B talked")
		}
		wa.Write(p)
		return wc.Write(p)
	})

	sa := newMockSerial(ra, wfa)                              //server a connection
	sb := NewFailbackConn(newMockSerial(rb, wfb), true, true) //server b connection
	cc := newMockSerial(rc, wfc)                              //client connection

	serverA := NewRTUServer(sa, slaveID)
	serverB := NewRTUServer(sb, slaveID)
	client := NewRTUCLient(cc, slaveID)

	_, shA, countA := newTestHandler("server A")
	_, shB, countB := newTestHandler("server B")
	holdingRegistersC, shC, countC := newTestHandler("client")
	for i := range holdingRegistersC {
		holdingRegistersC[i] = uint16(i + 1<<8)
	}

	go serverA.Serve(shA)
	go serverB.Serve(shB)
	go client.Serve(shC)
	return client, countA, countB, countC
}

func TestFailback(t *testing.T) {
	id := byte(3)
	client, countA, countB, countC := connectToMockServers(id)
	type tc struct {
		fc   FunctionCode
		size uint16
	}
	testCases := []tc{
		{FcWriteSingleRegister, 2},
		{FcWriteMultipleRegisters, 2},
		{FcReadHoldingRegisters, 2},
	}
	exCount := counter{}
	_ = os.Stdout
	//DebugOut = os.Stdout
	for i, ts := range testCases {
		t.Run(fmt.Sprintf("%v fc:%v size:%v", i, ts.fc, ts.size), func(t *testing.T) {
			reqs, err := MakePDURequestHeadersSized(ts.fc, 0, ts.size, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DoTransactions(client, id, reqs)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Second / 100)
			if ts.fc.IsWriteToServer() {
				exCount.writes += int(ts.size)
			} else {
				exCount.reads += int(ts.size)
			}
			if exCount.reads != countC.writes || exCount.writes != countC.reads {
				t.Error("client counter:", countC, "expected (inverted):", exCount)
			}
			if exCount.reads != countA.reads || exCount.writes != countA.writes {
				t.Error("server a counter:", countA, "expected:", exCount)
			}
			if exCount.reads != countB.reads || exCount.writes != countB.writes {
				t.Error("server b counter:", countB, "expected:", exCount)
			}
			exCount.reset()
			countA.reset()
			countB.reset()
			countC.reset()
		})
	}
}
