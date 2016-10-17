package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
)

type RTUServer struct {
	com     SerialPort
	SlaveId byte
}

const MaxRTUSize = 256

//Serve runs the server and only returns after unrecoverable error, such as com is closed.
//Right now Read is assumed to only read full packets, as per RTU delay based spec.
func (s *RTUServer) Serve(handler ProtocalHandler) error {
	delay := RTUMinDelay(s.com.BaudRate)

	rb := make([]byte, MaxRTUSize)
	var p PDU

	var ioerr error //make continue do io error checking
	wp := func(pdu PDU) {
		time.Sleep(delay)
		_, ioerr = s.com.Write(MakeRTU(s.SlaveId, pdu))
	}
	wec := func(err error) {
		wp(ErrorReplyPacket(p, ToExceptionCode(err)))
	}

	for ioerr == nil {
		var n int
		n, ioerr = s.com.Read(rb)
		if ioerr != nil {
			return ioerr
		}
		r := RTU(rb[:n])
		debugf("RTUServer read packet:%v\n", hex.EncodeToString(r))
		p, err := r.GetPDU()
		if err != nil {
			debugf("RTUServer drop read packet:%v\n", err)
			continue
		}
		if r[0] != 0 && r[0] != s.SlaveId {
			debugf("RTUServer drop packet to other id:%v\n", r[0])
			continue
		}
		ec := p.Validate()
		if ec != EcOK {
			debugf("RTUServer auto return for error code:%v\n", ec)
			wec(ec)
			continue
		}
		fc := p.GetFunctionCode()
		var out PDU
		if fc.ReadToServer() {
			out, err = handler.OnRead(p)
			if err != nil {
				debugf("RTUServer handler.OnOutput error:%v\n", err)
				wec(err)
				continue
			}
		} else if fc.WriteToServer() {
			data, err := p.GetRequestValues()
			if err != nil {
				debugf("RTUServer p.GetRequestValues error:%v\n", err)
				wec(err)
				continue
			}
			err = handler.OnWrite(p, data)
			if err != nil {
				debugf("RTUServer handler.OnInput error:%v\n", err)
				wec(err)
				continue
			}
			out = p.RepToWrite()
		}
		if !r.IsMulticast() {
			wp(out)
		}
	}
	return ioerr
}

func (s *RTUServer) IsClient() bool {
	return false
}

func (s *RTUServer) StartTransaction(req PDU) error {
	return errors.New("transactions can only be started from the client side")
}

//where to print debug messages
var DebugOut = os.Stdout

func debugf(format string, a ...interface{}) {
	fmt.Fprintf(DebugOut, "[%v]", time.Now())
	fmt.Fprintf(DebugOut, format, a...)
}
