package modbusone

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func connectMockClients(slaveID byte) (*RTUClient, *RTUClient, *counter, *counter, *counter) {

	//pipes
	ra, wa := io.Pipe() //client a
	rb, wb := io.Pipe() //client b
	rc, wc := io.Pipe() //server

	//everyone writes to everyone else
	wfa := io.MultiWriter(wb, wc) //write from a, etc...
	wfb := io.MultiWriter(wa, wc)
	wfc := io.MultiWriter(wa, wb)

	ca := NewFailoverConn(newMockSerial(ra, wfa), false, false) //client a connection
	cb := NewFailoverConn(newMockSerial(rb, wfb), true, false)  //client b connection
	sc := newMockSerial(rc, wfc)                                //server connection

	clientA := NewRTUClient(ca, slaveID)
	clientA.SkipTransactionCheck = true
	clientB := NewRTUClient(cb, slaveID)
	clientB.SkipTransactionCheck = true
	server := NewRTUServer(sc, slaveID)

	//faster timeouts during testing
	clientA.SetServerProcessingTime(time.Second / 50)
	clientB.SetServerProcessingTime(time.Second / 50)
	setDelays(ca)
	setDelays(cb)

	_, shA, countA := newTestHandler("client A")
	countA.Stats = ca.Stats()
	_, shB, countB := newTestHandler("client B")
	countB.Stats = cb.Stats()
	holdingRegistersC, shC, countC := newTestHandler("server")
	countC.Stats = sc.Stats()
	for i := range holdingRegistersC {
		holdingRegistersC[i] = uint16(i + 1<<8)
	}

	go clientA.Serve(shA)
	go clientB.Serve(shB)
	go server.Serve(shC)

	primaryActiveClient = func() bool {
		if ca.isActive {
			return true
		}
		ca.isActive = true
		ca.misses = ca.MissesMax
		return false
	}

	return clientA, clientB, countA, countB, countC
}

//return if primary is active, or set it to active is not already
var primaryActiveClient func() bool

func TestFailoverClient(t *testing.T) {
	id := byte(0x77)
	clientA, clientB, countA, countB, countC := connectMockClients(id)
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
	SetDebugOut(os.Stdout)
	defer func() { SetDebugOut(nil) }()

	t.Run("cold start", func(t *testing.T) {
		reqs, err := MakePDURequestHeadersSized(FcReadHoldingRegisters, 0, 1, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = DoTransactions(clientA, id, reqs)
		if err == nil {
			t.Fatal("cold start, not expecting any active clients")
		}
		t.Log("expected error:", err)
		_, err = DoTransactions(clientB, id, reqs)
		if err == nil {
			t.Fatal("cold start, not expecting any active clients")
		}
		for i := 0; i < 5; /*MissesMax*/ i++ {
			//activates client
			DoTransactions(clientA, id, reqs)
			DoTransactions(clientB, id, reqs)
		}
		if !primaryActiveClient() {
			t.Fatal("primaray servers should be active")
		}
		resetCounts()
	})
	//primaryActiveClient()

	for i, ts := range testCases {
		t.Run(fmt.Sprintf("normal %v fc:%v size:%v", i, ts.fc, ts.size), func(t *testing.T) {
			reqs, err := MakePDURequestHeadersSized(ts.fc, 0, ts.size, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			go DoTransactions(clientB, id, reqs)
			_, err = DoTransactions(clientA, id, reqs)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Second / 100)
			if ts.fc.IsReadToServer() {
				exCount.writes += int(ts.size)
			} else {
				exCount.reads += int(ts.size)
			}
			if exCount.reads != countC.writes || exCount.writes != countC.reads {
				t.Error("server counter     ", countC)
				t.Error("expected (inverted)", exCount)
				t.Error(countC.Stats)
			}
			if exCount.reads != countA.reads || exCount.writes != countA.writes {
				t.Error("client a counter", countA)
				t.Error("expected        ", exCount)
				t.Error(countA.Stats)
			}
			if exCount.reads != countB.reads || exCount.writes != countB.writes {
				t.Error("client b counter", countB)
				t.Error("expected        ", exCount)
				t.Error(countB.Stats)
			}
			resetCounts()
		})
	}
}
