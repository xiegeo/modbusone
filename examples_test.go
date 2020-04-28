//go:generate embedmd -w README.md
//go:generate sed -i -e 's/\t/\ \ \ \ /g' README.md

package modbusone_test

import (
	"fmt"
	"io"
	"net"

	"github.com/xiegeo/modbusone"
)

// handlerGenerator returns ProtocolHandlers that interact with our application.
// In this example, we are only using Holding Registers.
func handlerGenerator(name string) modbusone.ProtocolHandler {
	return &modbusone.SimpleHandler{
		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("%v ReadHoldingRegisters from %v, quantity %v\n",
				name, address, quantity)
			r := make([]uint16, quantity)
			// application code that fills in r here
			return r, nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("%v WriteHoldingRegisters from %v, quantity %v\n",
				name, address, len(values))
			// application code here
			return nil
		},
		OnErrorImp: func(req modbusone.PDU, errRep modbusone.PDU) {
			fmt.Printf("%v received error:%x in request:%x", name, errRep, req)
		},
	}
}

func Example_serialPort() {
	// Server id and baudRate, for Modbus over serial port.
	id := byte(1)
	baudRate := int64(19200)

	// Open serial connections:
	clientSerial, serverSerial := newInternalSerial()
	// Normally we want to open a serial connection from serial.OpenPort
	// such as github.com/tarm/serial. modbusone can take any io.ReadWriteCloser,
	// so we created two that talks to each other for demonstration here.

	// SerialContext adds baudRate information to calculate
	// the duration that data transfers should takes.
	// It also records Stats of read and dropped packets.
	clientSerialContext := modbusone.NewSerialContext(clientSerial, baudRate)
	serverSerialContext := modbusone.NewSerialContext(serverSerial, baudRate)

	// You can create either a client or a server from a SerialContext and an id.
	client := modbusone.NewRTUClient(clientSerialContext, id)
	server := modbusone.NewRTUServer(serverSerialContext, id)

	useClientAndServer(client, server, id) //follow the next function

	//Output:
	//reqs count: 2
	//reqs count: 3
	//server ReadHoldingRegisters from 0, quantity 125
	//client WriteHoldingRegisters from 0, quantity 125
	//server ReadHoldingRegisters from 125, quantity 75
	//client WriteHoldingRegisters from 125, quantity 75
	//client ReadHoldingRegisters from 1000, quantity 100
	//server WriteHoldingRegisters from 1000, quantity 100
	//server ReadHoldingRegisters from 0, quantity 125
	//client WriteHoldingRegisters from 0, quantity 125
	//server ReadHoldingRegisters from 125, quantity 75
	//client WriteHoldingRegisters from 125, quantity 75
	//client ReadHoldingRegisters from 1000, quantity 100
	//server WriteHoldingRegisters from 1000, quantity 100
	//serve terminated: io: read/write on closed pipe
}

func useClientAndServer(client modbusone.Client, server modbusone.ServerCloser, id byte) {
	termChan := make(chan error)

	// Serve is blocking until the serial connection has io errors or is closed.
	// So we use a goroutine to start it and continue setting up our demo.
	go client.Serve(handlerGenerator("client"))
	go func() {
		//A server is Started to same way as a client
		err := server.Serve(handlerGenerator("server"))
		// Do something with the err here.
		// For a command line app, you probably want to terminate.
		// For a service, you probably want to wait until you can open the serial port again.
		termChan <- err
	}()
	defer client.Close()
	defer server.Close()

	// If you only need to support server side, then you are done.
	// If you need to support client side, then you need to make requests.
	clientDoTransactions(client, id) //see following function

	// Clean up
	server.Close()
	fmt.Println("serve terminated:", <-termChan)
}

func clientDoTransactions(client modbusone.Client, id byte) {
	startAddress := uint16(0)
	quantity := uint16(200)
	reqs, err := modbusone.MakePDURequestHeaders(modbusone.FcReadHoldingRegisters,
		startAddress, quantity, nil)
	if err != nil {
		fmt.Println(err) //if what you asked for is not possible.
	}
	// Larger than allowed requests are split to many packets.
	fmt.Println("reqs count:", len(reqs))

	// We can add more requests, even of different types.
	// The last nil is replaced by the reqs to append to.
	startAddress = uint16(1000)
	quantity = uint16(100)
	reqs, err = modbusone.MakePDURequestHeaders(modbusone.FcWriteMultipleRegisters,
		startAddress, quantity, reqs)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("reqs count:", len(reqs))

	// Range over the requests to handle each individually,
	for _, r := range reqs {
		err = client.DoTransaction(r)
		if err != nil {
			fmt.Println(err, "on", r) // The server timed out, or the connection was closed.
		}
	}
	// or just do them all at once. Notice that reqs can be reused.
	n, err := modbusone.DoTransactions(client, id, reqs)
	if err != nil {
		fmt.Println(err, "on", reqs[n])
	}
} //end readme example marker

func Example_tcp() {
	// TCP address of the host
	host := "127.2.9.1:12345"

	// Default server id
	id := byte(1)

	// Open server tcp listner:
	listener, err := net.Listen("tcp", host)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Connect to server:
	conn, err := net.Dial("tcp", host)
	if err != nil {
		fmt.Println(err)
		return
	}

	// You can create either a client or a server
	client := modbusone.NewTCPClient(conn, 0)
	server := modbusone.NewTCPServer(listener)

	//shared example code with serial port
	useClientAndServer(client, server, id)

	//Output:
	//reqs count: 2
	//reqs count: 3
	//server ReadHoldingRegisters from 0, quantity 125
	//client WriteHoldingRegisters from 0, quantity 125
	//server ReadHoldingRegisters from 125, quantity 75
	//client WriteHoldingRegisters from 125, quantity 75
	//client ReadHoldingRegisters from 1000, quantity 100
	//server WriteHoldingRegisters from 1000, quantity 100
	//server ReadHoldingRegisters from 0, quantity 125
	//client WriteHoldingRegisters from 0, quantity 125
	//server ReadHoldingRegisters from 125, quantity 75
	//client WriteHoldingRegisters from 125, quantity 75
	//client ReadHoldingRegisters from 1000, quantity 100
	//server WriteHoldingRegisters from 1000, quantity 100
	//serve terminated: accept tcp 127.2.9.1:12345: use of closed network connection
}

type serial struct {
	io.ReadCloser
	io.WriteCloser
}

func newInternalSerial() (io.ReadWriteCloser, io.ReadWriteCloser) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	return &serial{ReadCloser: r1, WriteCloser: w2}, &serial{ReadCloser: r2, WriteCloser: w1}
}
func (s *serial) Close() error {
	s.ReadCloser.Close()
	return s.WriteCloser.Close()
}
