package modbusone_test

import (
	"fmt"
	"io"

	"github.com/xiegeo/modbusone"
)

func Example() {

	// Server id and baudRate, two critical information you need for modbus over serial port.
	id := byte(1)
	baudRate := int64(19200)

	// Open serial connections:
	clientSerial, serverSerial := newInternalSerial()
	// Normally we want to open a serial connetion from serial.OpenPort
	// such as github.com/tarm/serial.
	// modbusone can take any io.ReadWriteCloser, so we created two that talks to each other
	// for demonstration here.

	// SerialContext adds baudRate information to calculate the duration that data transfers should takes.
	// It also records Stats of read and dropped packets
	clientSerialContext := modbusone.NewSerialContext(clientSerial, baudRate)
	serverSerialContext := modbusone.NewSerialContext(serverSerial, baudRate)

	// You can create either a client or a server from a SerialContext and an id
	client := modbusone.NewRTUClient(clientSerialContext, id)
	server := modbusone.NewRTUClient(serverSerialContext, id)

	// Create Handler to handle client and server actions, in this example, we are only using Holding Registers
	handler := &modbusone.SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("ReadHoldingRegisters from %v, quantity %v\n", address, quantity)
			r := make([]uint16, quantity)
			// application code that fills in r here
			return r, nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("WriteHoldingRegisters from %v, quantity %v\n", address, len(values))
			// application code here
			return nil
		},
		OnErrorImp: func(req modbusone.PDU, errRep modbusone.PDU) {
			fmt.Printf("client handler received error:%x in request:%x", errRep, req)
		},
	}

	// Now we are ready to serve!
	// Serve is blocking until the serial connection has errors or is closed.
	go client.Serve(handler)
	go func() {
		err := server.Serve(handler)
		_ = err
		// do something with the err here. For a commandline app, you probably want to terminate.
		// For a demon, you probably want to wait until you can open the serial port again.
	}()
	defer client.Close()
	defer server.Close()
	// You only need to call close if you need to close or reuse a working connection without restarting the process

	// If you only need to support server side, then you are done.
	// If you need to supoort client side, then you need to make requests.

	//Output:
}

type serial struct {
	io.Reader
	io.Writer
	closers []io.Closer
}

func newInternalSerial() (io.ReadWriteCloser, io.ReadWriteCloser) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	cs := []io.Closer{r1, r2}
	return &serial{Reader: r1, Writer: w2, closers: cs}, &serial{Reader: r2, Writer: w1, closers: cs}
}
func (s *serial) Close() error {
	for _, c := range s.closers {
		c.Close()
	}
	return nil
}
