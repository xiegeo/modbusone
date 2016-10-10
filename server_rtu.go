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
	delay := 1750 * time.Microsecond
	if s.com.BaudRate <= 19200 {
		br := time.Duration(s.com.BaudRate)
		oneBitDuration := (time.Second + br - 1) / br //time it takes to send a bit
		delay = (oneBitDuration*11*7 + 1) / 2         //time it takes to send 3.5 chars (a char of 8 takes 11 bits on wire)
	}

	rb := make([]byte, MaxRTUSize)
	var p PDU

	wp := func(pdu PDU) error {
		time.Sleep(delay)
		_, err := s.com.Write(MakeRTU(pdu))
		return err
	}
	wec := func(ec ExceptionCode) error {
		return wp(ErrorReplyPacket(p, ec))
	}
	var ioerr error //make continue do io error checking
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
			ioerr = wec(ec)
			continue
		}
		fc := p.GetFunctionCode()
		if fc.ReadToServer() {
			var out PDU
			out, err = handler.OnOutput(p)
			if err != nil {
				debugf("RTUServer handler.OnOutput error:%v\n", err)
				ioerr = wec(EcServerDeviceFailure)
				continue
			}
			ioerr = wp(out)
			continue
		}
		if fc.WriteToServer() {
			err = handler.OnInput(p)
			if err != nil {
				debugf("RTUServer handler.OnInput error:%v\n", err)
				ioerr = wec(EcServerDeviceFailure)
				continue
			}
			ioerr = wp(p.RepToWrite())
			continue
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
