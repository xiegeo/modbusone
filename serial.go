package modbusone

import (
	"fmt"
	"io"
	"time"

	burrowSerial "github.com/goburrow/serial"
)

//SerialContext is an interface implemented by SerialPort, can also be mocked for testing.
type SerialContext interface {
	io.ReadWriteCloser
	//RTUMinDelay returns the minimum required delay between packets for framing
	MinDelay() time.Duration
	//RTUBytesDelay returns the duration is takes to send n bytes
	BytesDelay(n int) time.Duration
}

type serial struct {
	conn     io.ReadWriteCloser
	baudRate int64
}

//NewSerialContext creates a SerialContext from any io.ReadWriteCloser
func NewSerialContext(conn io.ReadWriteCloser, baudRate int64) SerialContext {
	return &serial{conn, baudRate}
}

//Read reads the serial port and removes the timeout error
func (s *serial) Read(b []byte) (int, error) {
	n, err := s.conn.Read(b)
	return n, err
}

func (s *serial) Write(b []byte) (int, error) {
	debugf("SerialPort Write:%x\n", b)
	n, err := s.conn.Write(b)
	return n, err
}

func (s *serial) Close() error {
	return s.conn.Close()
}

func (s *serial) MinDelay() time.Duration {
	return MinDelay(int64(s.baudRate))
}

func (s *serial) BytesDelay(n int) time.Duration {
	return BytesDelay(int64(s.baudRate), n)
}

//SerialPort has configuration and I/O controller.
//It is backed by github.com/goburrow/serial.
//
//Please use NewSerialContext instead, which support any io.ReadWriteCloser
type SerialPort struct {
	// Serial port configuration.
	burrowSerial.Config
	// port is a platform-dependent data structure for serial port.
	port burrowSerial.Port

	isConnected bool
}

//Open opens the SerialPort with Config
func (s *SerialPort) Open(c burrowSerial.Config) (err error) {
	if s.isConnected {
		return fmt.Errorf("already opened")
	}
	s.Config = c
	if s.port == nil {
		s.port, err = burrowSerial.Open(&s.Config)
	} else {
		//reuse closed port
		err = s.port.Open(&s.Config)
	}
	s.isConnected = err == nil
	return
}

//Read reads the serial port and removes the timeout error
func (s *SerialPort) Read(b []byte) (int, error) {
	n, err := s.port.Read(b)
	if err == burrowSerial.ErrTimeout {
		debugf("serial read timeout")
		return n, nil
	}
	if n == 0 && err == nil {
		debugf("serial read nil")
		return n, fmt.Errorf("serial read nil")
	}
	return n, err
}
func (s *SerialPort) Write(b []byte) (int, error) {
	debugf("SerialPort Write:%x\n", b)
	n, err := s.port.Write(b)
	return n, err
}

//Close closes the SerialPort
func (s *SerialPort) Close() error {
	s.isConnected = false
	return s.port.Close()
}

//MinDelay returns the minum Delay of 3.5 bytes between packets or 1750 mircos
func (s *SerialPort) MinDelay() time.Duration {
	return MinDelay(int64(s.BaudRate))
}

//MinDelay returns the minum Delay of 3.5 bytes between packets or 1750 mircos
func MinDelay(baudRate int64) time.Duration {
	delay := 1750 * time.Microsecond
	br := time.Duration(baudRate)
	if br <= 19200 {
		//time it takes to send 3.5 or 7/2 chars (a char of 8 bits takes 11 bits on wire)
		delay = (time.Second*11*7 + (br * 2) - 1) / (br * 2)
	}
	return delay
}

//BytesDelay returns the time it takes to send n bytes in baudRate
func (s *SerialPort) BytesDelay(n int) time.Duration {
	return BytesDelay(int64(s.BaudRate), n)
}

//BytesDelay returns the time it takes to send n bytes in baudRate
func BytesDelay(baudRate int64, n int) time.Duration {
	br := time.Duration(baudRate)
	cs := time.Duration(n)

	return (time.Second*11*cs + br - 1) / br
}
