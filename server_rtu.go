package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

type RTUServer struct {
	com     SerialContext
	SlaveID byte
}

const MaxRTUSize = 256

//NewRTUServer creates a RTU server on SerialContext listening on slaveID
func NewRTUServer(com SerialContext, slaveID byte) *RTUServer {
	r := RTUServer{
		com:     com,
		SlaveID: slaveID,
	}
	return &r
}

//Serve runs the server and only returns after unrecoverable error, such as
//SerialContext is closed. Read is assumed to only read full packets,
//as per RTU delay based spec.
func (s *RTUServer) Serve(handler ProtocalHandler) error {
	delay := s.com.MinDelay()

	rb := make([]byte, MaxRTUSize)
	var p PDU

	var ioerr error //make continue do io error checking
	wp := func(pdu PDU, slaveId byte) {
		if slaveId == 0 {
			return
		}
		time.Sleep(delay)
		_, ioerr = s.com.Write(MakeRTU(slaveId, pdu))
	}
	wec := func(err error, slaveId byte) {
		wp(ExceptionReplyPacket(p, ToExceptionCode(err)), slaveId)
	}

	for ioerr == nil {
		var n int
		debugf("RTUServer wait for read\n")
		n, ioerr = s.com.Read(rb)
		if ioerr != nil {
			return ioerr
		}
		r := RTU(rb[:n])
		debugf("RTUServer read packet:%v\n", hex.EncodeToString(r))
		var err error
		p, err = r.GetPDU()
		if err != nil {
			debugf("RTUServer drop read packet:%v\n", err)
			continue
		}
		if r[0] != 0 && r[0] != s.SlaveID {
			debugf("RTUServer drop packet to other id:%v\n", r[0])
			continue
		}
		err = p.ValidateRequest()
		if err != nil {
			debugf("RTUServer auto return for error:%v\n", err)
			wec(err, r[0])
			continue
		}
		fc := p.GetFunctionCode()
		if fc.ReadToServer() {
			data, err := handler.OnRead(p)
			if err != nil {
				debugf("RTUServer handler.OnOutput error:%v\n", err)
				wec(err, r[0])
				continue
			}
			wp(p.MakeReadReply(data), r[0])
		} else if fc.WriteToServer() {
			data, err := p.GetRequestValues()
			if err != nil {
				debugf("RTUServer p.GetRequestValues error:%v\n", err)
				wec(err, r[0])
				continue
			}
			err = handler.OnWrite(p, data)
			if err != nil {
				debugf("RTUServer handler.OnInput error:%v\n", err)
				wec(err, r[0])
				continue
			}
			wp(p.MakeWriteReply(), r[0])
		}
	}
	return ioerr
}

//Uint64ToSlaveID is a helper function for reading configuration data to SlaveID.
//See also flag.Uint64 and strcov.ParseUint
func Uint64ToSlaveID(n uint64) (byte, error) {
	if n > 247 {
		return 0, errors.New("slaveID must be less than 248")
	}
	return byte(n), nil
}

//DebugOut sets where to print debug messages
var DebugOut io.Writer = nil

func debugf(format string, a ...interface{}) {
	if DebugOut == nil {
		return
	}
	fmt.Fprintf(DebugOut, "[%s]", time.Now().Format("06-01-02 15:04:05.000000"))
	fmt.Fprintf(DebugOut, format, a...)
	lf := len(format)
	if lf > 0 && format[lf-1] != '\n' {
		fmt.Fprintln(DebugOut)
	}
}
