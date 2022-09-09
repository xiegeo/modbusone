package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"sync/atomic"
	"time"
)

// RTUServer implements Server/Slave side logic for RTU over a SerialContext to
// be used by a ProtocolHandler.
type RTUServer struct {
	com          SerialContext
	packetReader PacketReader
	SlaveID      byte
}

// NewRTUServer creates a RTU server on SerialContext listening on slaveID.
func NewRTUServer(com SerialContext, slaveID byte) *RTUServer {
	pr, ok := com.(PacketReader)
	if !ok {
		pr = NewRTUPacketReader(com, false)
	}
	r := RTUServer{
		com:          com,
		packetReader: pr,
		SlaveID:      slaveID,
	}
	return &r
}

// Serve runs the server and only returns after unrecoverable error, such as
// SerialContext is closed.
func (s *RTUServer) Serve(handler ProtocolHandler) error {
	defer s.Close()

	delay := s.com.MinDelay()

	var rb []byte
	if OverSizeSupport {
		rb = make([]byte, OverSizeMaxRTU)
	} else {
		rb = make([]byte, MaxRTUSize)
	}

	var p PDU

	var ioErr error // make continue do io error checking
	wp := func(pdu PDU, slaveId byte) {
		if slaveId == 0 {
			return
		}
		time.Sleep(delay)
		_, ioErr = s.com.Write(MakeRTU(slaveId, pdu))
	}
	wec := func(err error, slaveId byte) {
		wp(ExceptionReplyPacket(p, ToExceptionCode(err)), slaveId)
	}

	for ioErr == nil {
		var n int
		debugf("RTUServer wait for read\n")
		n, ioErr = s.packetReader.Read(rb)
		if ioErr != nil {
			return ioErr
		}
		r := RTU(rb[:n])
		debugf("RTUServer read packet:%v\n", hex.EncodeToString(r))
		var err error
		p, err = r.GetPDU()
		if err != nil {
			if errors.Is(err, ErrorCrc) {
				atomic.AddInt64(&s.com.Stats().CrcErrors, 1)
			} else {
				atomic.AddInt64(&s.com.Stats().OtherErrors, 1)
			}
			debugf("RTUServer drop read packet:%v\n", err)
			continue
		}
		if r[0] != 0 && r[0] != s.SlaveID {
			atomic.AddInt64(&s.com.Stats().IDDrops, 1)
			debugf("RTUServer drop packet to other id:%v\n", r[0])
			continue
		}
		err = p.ValidateRequest()
		if err != nil {
			atomic.AddInt64(&s.com.Stats().OtherErrors, 1)
			debugf("RTUServer auto return for error:%v\n", err)
			wec(err, r[0])
			continue
		}
		fc := p.GetFunctionCode()
		if fc.IsReadToServer() {
			data, err := handler.OnRead(p)
			if err != nil {
				atomic.AddInt64(&s.com.Stats().OtherErrors, 1)
				debugf("RTUServer handler.OnOutput error:%v\n", err)
				wec(err, r[0])
				continue
			}
			wp(p.MakeReadReply(data), r[0])
		} else if fc.IsWriteToServer() {
			data, err := p.GetRequestValues()
			if err != nil {
				atomic.AddInt64(&s.com.Stats().OtherErrors, 1)
				debugf("RTUServer p.GetRequestValues error:%v\n", err)
				wec(err, r[0])
				continue
			}
			err = handler.OnWrite(p, data)
			if err != nil {
				atomic.AddInt64(&s.com.Stats().OtherErrors, 1)
				debugf("RTUServer handler.OnInput error:%v\n", err)
				wec(err, r[0])
				continue
			}
			wp(p.MakeWriteReply(), r[0])
		}
	}
	return ioErr
}

// Close closes the server and closes the connect.
func (s *RTUServer) Close() error {
	return s.com.Close()
}

// Uint64ToSlaveID is a helper function for reading configuration data to SlaveID.
// See also flag.Uint64 and strconv.ParseUint.
func Uint64ToSlaveID(n uint64) (byte, error) {
	if n > 247 {
		return 0, errors.New("slaveID must be less than 248")
	}
	return byte(n), nil
}

// debugOutput sets where to print debug messages.
var debugOutput atomic.Value

// SetDebugOut to print debug messages, set to nil to turn off debug output.
func SetDebugOut(w io.Writer) {
	debugOutput.Store(&w) // nil can not be store to atomic.Value directly, but a pointer to nil can.
}

const monkey = false

func debugf(format string, a ...interface{}) {
	if monkey && rand.Float32() < 0.5 { //nolint:gosec // Monkey testing's random doesn't need secure random numbers.
		runtime.Gosched()
	}
	debugWriterP, _ := debugOutput.Load().(*io.Writer)
	if debugWriterP == nil || *debugWriterP == nil {
		// SetDebugOut is never called, or last set to nil
		return
	}
	debugWriter := *debugWriterP
	fmt.Fprintf(debugWriter, "[%s]", time.Now().Format("06-01-02 15:04:05.000000"))
	fmt.Fprintf(debugWriter, format, a...)
	lf := len(format)
	if lf > 0 && format[lf-1] != '\n' {
		// make sure each call to debugf ends in new line.
		fmt.Fprintln(debugWriter)
	}
}
