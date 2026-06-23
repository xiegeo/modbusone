package modbusone_test

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	. "github.com/xiegeo/modbusone"
)

func TestMultiServer(t *testing.T) {
	//	SetDebugOut(coloredgoroutine.Colors(os.Stdout))
	//	defer func() {
	//		SetDebugOut(nil)
	//	}()

	servers := byte(2)
	nodes := connectMock2wNodes(t, int(servers))
	defer func() {
		nodes.client.Close()
		for _, s := range nodes.servers {
			s.Close()
		}
	}()

	// writes data
	for i := byte(1); i <= servers; i++ {
		nodes.getClientHandLer(i).holdingRegisters[0] = uint16(i)
	}
	pdu, err := MakePDURequestHeaders(FcWriteMultipleRegisters, 0, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := byte(1); i <= servers; i++ {
		err = DoRTUTransaction(nodes.client, RTUHeader{SlaveID: i, PDU: pdu[0]})
		if err != nil {
			t.Fatal("SlaveID:", i, " error:", err)
		}
	}

	// over write client memory
	for i := byte(1); i <= servers; i++ {
		nodes.getClientHandLer(i).holdingRegisters[0] = 9
	}

	// retrieve data
	pdu, err = MakePDURequestHeaders(FcReadHoldingRegisters, 0, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := byte(1); i <= servers; i++ {
		err = DoRTUTransaction(nodes.client, RTUHeader{SlaveID: i, PDU: pdu[0]})
		if err != nil {
			t.Fatal(i, err)
		}
	}
	for i := byte(1); i <= servers; i++ {
		require.Equal(t, uint16(i), nodes.getClientHandLer(i).holdingRegisters[0])
	}
}

type mockNodes struct {
	client         *RTUClient
	servers        []*RTUServer
	multiIDHandler MultiIDHandler
}

func (m *mockNodes) getClientHandLer(slaveID byte) *testHandler {
	return m.multiIDHandler[slaveID].(*testHandler)
}

func connectMock2wNodes(t *testing.T, slaves int) *mockNodes {
	// pipes
	rs := make([]*io.PipeReader, 1+slaves)
	ws := make([]io.Writer, 1+slaves)
	for i := 0; i < 1+slaves; i++ {
		rs[i], ws[i] = io.Pipe()
	}

	// everyone writes to everyone else
	wfs := make([]io.Writer, 1+slaves)
	for i := 0; i < 1+slaves; i++ {
		wfs[i] = io.MultiWriter(append(append([]io.Writer{}, ws[:i]...), ws[i+1:]...)...)
	}

	// start servers
	servers := make([]*RTUServer, slaves)
	multiIDHandler := MultiIDHandler{}
	for i := 1; i < 1+slaves; i++ {
		serialContext := newMockSerial(t, fmt.Sprintf("s%v", i), rs[i], wfs[i], rs[i])
		serialContext.TwoWire = true
		s := NewRTUServer(serialContext, byte(i))
		servers[i-1] = s
		h := newTestHandler2(fmt.Sprintf("sh%v", i), t)
		go s.Serve(h)
		multiIDHandler[byte(i)] = newTestHandler2(fmt.Sprintf("ch%v", i), t)
	}

	//start client
	client := NewRTUClient(newMockSerial(t, "client", rs[0], wfs[0], rs[0]), 0)
	go client.ServeRTU(multiIDHandler)

	return &mockNodes{
		client:         client,
		servers:        servers,
		multiIDHandler: multiIDHandler,
	}
}
