package modbusone

import (
	"fmt"
	"io"
	"net"
)

const (
	TCPHeaderLength  = 6
	MBAPHeaderLength = TCPHeaderLength + 1
)

// TCPServer implements Server/Slave side logic for Modbus over TCP to
// be used by a ProtocolHandler.
type TCPServer struct {
	listener net.Listener
}

// NewTCPServer runs TCP server.
func NewTCPServer(listener net.Listener) *TCPServer {
	s := TCPServer{
		listener: listener,
	}
	return &s
}

func readTCP(r io.Reader, bs []byte) (n int, err error) {
	n, err = io.ReadFull(r, bs[:TCPHeaderLength])
	if err != nil {
		return n, err
	}
	if bs[2] != 0 || bs[3] != 0 {
		return n, fmt.Errorf("MBAP protocol of %X %X is unknown", bs[2], bs[3])
	}
	l := int(bs[4])*256 + int(bs[5])
	if l <= 2 {
		return n, fmt.Errorf("MBAP data length of %v is too short, bs:%x", l, bs[:n])
	}
	if len(bs) < l+TCPHeaderLength {
		return n, fmt.Errorf("MBAP data length of %v is too long", l)
	}
	n, err = io.ReadFull(r, bs[TCPHeaderLength:l+TCPHeaderLength])
	return n + TCPHeaderLength, err
}

// writeTCP writes a PDU packet on TCP reusing the headers and buffer space in bs.
func writeTCP(w io.Writer, bs []byte, pdu PDU) (int, error) {
	l := len(pdu) + 1 // PDU + byte of slaveID
	bs[4] = byte(l / 256)
	bs[5] = byte(l)
	copy(bs[MBAPHeaderLength:], pdu)
	return w.Write(bs[:len(pdu)+MBAPHeaderLength])
}

// Serve runs the server and only returns after a connection or data error occurred.
// The underling connection is always closed before this function returns.
func (s *TCPServer) Serve(handler ProtocolHandler) error {
	defer s.Close()

	wec := func(conn net.Conn, bs []byte, req PDU, err error) {
		writeTCP(conn, bs, ExceptionReplyPacket(req, ToExceptionCode(err)))
	}

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go func(conn net.Conn) {
			defer conn.Close()

			var rb []byte
			if OverSizeSupport {
				rb = make([]byte, MBAPHeaderLength+OverSizeMaxRTU+TCPHeaderLength)
			} else {
				rb = make([]byte, MBAPHeaderLength+MaxPDUSize)
			}

			for {
				n, err := readTCP(conn, rb)
				if err != nil {
					debugf("readTCP %v\n", err)
					return
				}
				p := PDU(rb[MBAPHeaderLength:n])
				err = p.ValidateRequest()
				if err != nil {
					debugf("ValidateRequest %v\n", err)
					return
				}

				fc := p.GetFunctionCode()
				if fc.IsReadToServer() {
					data, err := handler.OnRead(p)
					if err != nil {
						debugf("TCPServer handler.OnOutput error:%v\n", err)
						wec(conn, rb, p, err)
						continue
					}
					writeTCP(conn, rb, p.MakeReadReply(data))
				} else if fc.IsWriteToServer() {
					data, err := p.GetRequestValues()
					if err != nil {
						debugf("p:%v\n", p)
						debugf("TCPServer p.GetRequestValues error:%v\n", err)
						wec(conn, rb, p, err)
						continue
					}
					err = handler.OnWrite(p, data)
					if err != nil {
						debugf("TCPServer handler.OnInput error:%v\n", err)
						wec(conn, rb, p, err)
						continue
					}
					writeTCP(conn, rb, p.MakeWriteReply())
				}
			}
		}(conn)
	}
}

// Close closes the server and closes the listener.
func (s *TCPServer) Close() error {
	return s.listener.Close()
}
