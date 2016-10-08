package modbusone

import (
	"fmt"
	"time"

	"github.com/goburrow/serial"
)

// SerialPort has configuration and I/O controller.
type SerialPort struct {
	// Serial port configuration.
	serial.Config
	// port is platform-dependent data structure for serial port.
	port serial.Port

	isConnected bool
}

func (s *SerialPort) Open(c serial.Config) (err error) {
	if s.isConnected {
		return fmt.Errorf("already opened")
	}
	s.Config = c
	if s.port == nil {
		s.port, err = serial.Open(&s.Config)
	} else {
		//reuse closed port
		err = s.port.Open(&s.Config)
	}
	s.isConnected = err == nil
	return
}

func (s *SerialPort) Close() (err error) {
	if !s.isConnected {
		return fmt.Errorf("already closed")
	}
	err = s.port.Close()
	s.isConnected = false
	return
}
