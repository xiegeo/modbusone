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

type counter struct {
	*Stats
	reads  int
	writes int
}

func (c *counter) reset() {
	c.Stats.Reset()
	c.reads = 0
	c.writes = 0
}

func (c counter) String() string {
	return fmt.Sprintf("reads:%v writes:%v drops:%v", c.reads, c.writes, c.TotalDrops())
}

func newTestHandler(name string, t *testing.T) ([]uint16, *SimpleHandler, *counter) {
	var holdingRegisters [100]uint16
	count := counter{}
	shA := &SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			t.Logf("Read %s %v, quantity %v\n", name, address, quantity)
			count.reads += int(quantity)
			return holdingRegisters[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			t.Logf("Write %s %v, quantity %v\n", name, address, len(values))
			count.writes += len(values)
			for i, v := range values {
				holdingRegisters[address+uint16(i)] = v
			}
			return nil
		},
	}
	return holdingRegisters[:], shA, &count
}

func setDelays(f *FailoverSerialConn) {
	f.SecondaryDelay = time.Second / 100
	f.MissDelay = time.Second / 50
}

func connectToMockServers(t *testing.T, slaveID byte) (*RTUClient, *counter, *counter, *counter) {

	//pipes
	ra, wa := io.Pipe() //server a
	rb, wb := io.Pipe() //server b
	rc, wc := io.Pipe() //client

	//everyone writes to everyone else
	wfa := io.MultiWriter(wb, wc) //write from a, etc...
	wfb := io.MultiWriter(wa, wc)
	wfc := io.MultiWriter(wa, wb)

	sa := NewFailoverConn(newMockSerial(ra, wfa), false, false) //server a connection
	sb := NewFailoverConn(newMockSerial(rb, wfb), true, false)  //server b connection
	cc := newMockSerial(rc, wfc)                                //client connection

	serverA := NewRTUServer(sa, slaveID)
	serverB := NewRTUServer(sb, slaveID)
	client := NewRTUClient(cc, slaveID)

	//faster timeouts during testing
	client.SetServerProcessingTime(time.Second / 10)
	setDelays(sa)
	setDelays(sb)

	_, shA, countA := newTestHandler("server A", t)
	countA.Stats = sa.Stats()
	_, shB, countB := newTestHandler("server B", t)
	countB.Stats = sb.Stats()
	holdingRegistersC, shC, countC := newTestHandler("client", t)
	countC.Stats = cc.Stats()
	for i := range holdingRegistersC {
		holdingRegistersC[i] = uint16(i + 1<<8)
	}

	go serverA.Serve(shA)
	go serverB.Serve(shB)
	go client.Serve(shC)

	primaryActiveServer = func() bool {
		if sa.isActive {
			return true
		}
		sa.isActive = true
		sa.misses = sa.MissesMax
		return false
	}

	return client, countA, countB, countC
}

//return if primary is active, or set it to active is not already
var primaryActiveServer func() bool

func TestFailoverServer(t *testing.T) {
	id := byte(0x77)
	client, countA, countB, countC := connectToMockServers(t, id)
	exCount := counter{Stats: &Stats{}}
	resetCounts := func() {
		exCount.reset()
		countA.reset()
		countB.reset()
		countC.reset()
	}

	type tc struct {
		fc   FunctionCode
		size uint16
	}
	testCases := []tc{
		{FcWriteSingleRegister, 20},
		{FcWriteMultipleRegisters, 20},
		{FcReadHoldingRegisters, 20},
	}

	_ = os.Stdout
	//SetDebugOut(os.Stdout)
	defer func() { SetDebugOut(nil) }()

	t.Run("cold start", func(t *testing.T) {
		reqs, err := MakePDURequestHeadersSized(FcReadHoldingRegisters, 0, 1, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = DoTransactions(client, id, reqs)
		if err == nil {
			t.Fatal("cold start, not expecting any active servers")
		}
		for i := 0; i < 5; /*ServerMissesMax*/ i++ {
			//activates server
			DoTransactions(client, id, reqs)
		}
		if !primaryActiveServer() {
			t.Fatal("primaray servers should be active")
		}
		resetCounts()
	})
	primaryActiveServer()

	for i, ts := range testCases {
		t.Run(fmt.Sprintf("normal %v fc:%v size:%v", i, ts.fc, ts.size), func(t *testing.T) {
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
				t.Error("client counter     ", countC)
				t.Error("expected (inverted)", exCount)
			}
			if exCount.reads != countA.reads || exCount.writes != countA.writes {
				t.Error("server a counter", countA)
				t.Error("expected        ", exCount)
			}
			if exCount.reads != countB.reads || exCount.writes != countB.writes {
				t.Error("server b counter", countB)
				t.Error("expected        ", exCount)
			}
			resetCounts()
		})
	}
}
