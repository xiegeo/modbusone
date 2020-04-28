package modbusone

import (
	"fmt"
	"io"
	"net"
)

//RTUServer implements Server/Slave side logic for Modbus over TCP to
//be used by a ProtocolHandler
type TCPServer struct {
	listener net.Listener
}

//NewTCPServer runs TCP server
func NewTCPServer(listener net.Listener) *TCPServer {
	s := TCPServer{
		listener: listener,
	}
	return &s
}

func readTCP(r io.Reader, bs []byte) (n int, err error) {
	h := 6 // read MBAP Header until length
	n, err = io.ReadFull(r, bs[:h])
	if err != nil {
		return n, err
	}
	if bs[2] != 0 || bs[3] != 0 {
		return n, fmt.Errorf("MBAP protocol of %X %X is unknown", bs[2], bs[3])
	}
	l := int(bs[4])*256 + int(bs[5])
	if len(bs) < l+h {
		return n, fmt.Errorf("MBAP data length of %v is too long", l)
	}
	n, err = io.ReadFull(r, bs[h:l+h])
	return n + h, err
}

// Serve runs the server and only returns after a connetion or data error occurred.
// The underling connection is awalys closed before this fuction returns.
func (s *TCPServer) Serve(handler ProtocolHandler) error {
	defer s.Close()

	wp := func(conn net.Conn, bs []byte, pdu PDU) (int, error) {
		bs[4] = byte(len(pdu) / 256)
		bs[5] = byte(len(pdu))
		copy(bs[7:], pdu)
		return conn.Write(bs[:len(pdu)+7])
	}
	wec := func(conn net.Conn, bs []byte, req PDU, err error) {
		wp(conn, bs, ExceptionReplyPacket(req, ToExceptionCode(err)))
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
				rb = make([]byte, OverSizeMaxRTU+7)
			} else {
				rb = make([]byte, MaxRTUSize+7)
			}

			for {
				n, err := readTCP(conn, rb)
				if err != nil {
					debugf("readTCP %v\n", err)
					return
				}
				p := PDU(rb[6:n])
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
					wp(conn, rb, p.MakeReadReply(data))
				} else if fc.IsWriteToServer() {
					data, err := p.GetRequestValues()
					if err != nil {
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
					wp(conn, rb, p.MakeWriteReply())
				}
			}
		}(conn)
	}
}

//Close closes the server and closes the listener
func (s *TCPServer) Close() error {
	return s.listener.Close()
}
