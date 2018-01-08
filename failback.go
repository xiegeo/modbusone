package modbusone

import (
	"bytes"
	"errors"
	"io"
	"time"
)

var SecondaryDelay = time.Second / 10
var MissDelay = time.Second / 5 //must be bigger than SecondaryDelay for primary to detect failback

//FailbackSerialConn manages a failback connection, which does failback using
//shared serial bus and shared slaveId. Slaves using other ids on the same
//bus is not supported. If the other side supports multiple slave ids, then
//it is best to implement failback on the other side using slaveId method.
type FailbackSerialConn struct {
	serial       //base SerialContext
	packetReader io.Reader
	isServer     bool //client or server
	isFailback   bool //primary or failback
	isActive     bool //active or passive

	requestTime time.Time //time of the last packet observed passively
	reqPacket   PDU
	lastRead    time.Time

	//if primary has not recived data for this long, it thinks it's disconnected
	//and go passive, just like at restart
	//default 10 seconds
	PrimaryDisconnectDelay time.Duration

	//when a failback is running,
	//how long should it wait to take over again.
	//default 10 mins
	PrimaryForceBackDelay time.Duration
	startTime             time.Time

	//how many misses is the primary server is detected as down
	//default 5
	ServerMissesMax int
	serverMisses    int

	//how long untill the primary client is detected as down
	ClientMissing     time.Duration
	clientLastMessage time.Time
}

//NewSerialContext creates a SerialContext from any io.ReadWriteCloser
func NewFailbackServerlConn(conn io.ReadWriteCloser, baudRate int64, isFailback bool) *FailbackSerialConn {
	c := &FailbackSerialConn{
		serial:                 serial{conn, baudRate, Stats{}},
		isServer:               true,
		isFailback:             isFailback,
		PrimaryDisconnectDelay: 10 * time.Second,
		PrimaryForceBackDelay:  10 * time.Minute,
		startTime:              time.Now(),
		ServerMissesMax:        5,
	}
	c.packetReader = NewRTUBidirectionalPacketReader(&c.serial)
	return c
}

//Read reads the serial port
func (s *FailbackSerialConn) Read(b []byte) (int, error) {
	defer func() {
		s.lastRead = time.Now()
	}()
	if !s.isServer {
		return 0, errors.New("todo client failback")
	}
	for {
		n, err := s.packetReader.Read(b)
		if err != nil {
			return n, err
		}
		if !s.isFailback {
			if !s.isActive {
				if s.startTime.Add(s.PrimaryForceBackDelay).Before(time.Now()) {
					debugf("force active of primary/n")
					s.isActive = true
				}
			}
			if s.isActive {
				if s.lastRead.Add(s.PrimaryDisconnectDelay).Before(time.Now()) {
					debugf("primary was disconnected for too long/n")
					s.isActive = false
					s.startTime = time.Now()
				} else {
					return n, nil
				}
			}
		}

		rtu := RTU(b)
		pdu, err := rtu.GetPDU()
		if err != nil {
			debugf("GetPDU error : %v", err)
			return n, nil //bubbles formate up errors
		}
		if rtu[0] == 0 {
			//zero slave id do not have a reply, so we won't expect one
			s.resetRequestTime()
			return n, nil
		}
		if s.isActive {
			if !s.isFailback {
				return 0, errors.New("assert isFailback")
			}
			//are we getting interrupted?
			if s.requestTime.IsZero() {
				//this should be a client request
				s.requestTime = time.Now()
				return n, nil
			}
			//yes
			s.isActive = false
			s.serverMisses = 0
			s.resetRequestTime()
			debugf("primary found, going from active to passive")
			continue //throw away and read again

		} else {
			//we are passive here
			if s.requestTime.IsZero() {
				s.requestTime = time.Now()
				s.reqPacket = pdu
				return n, nil
			}
			now := time.Now()
			if now.Sub(s.requestTime) > MissDelay+s.BytesDelay(n) {
				s.requestTime = now
				s.serverMisses++
				if s.serverMisses > s.ServerMissesMax {
					s.isActive = true
				}
				return n, nil
			}

			s.serverMisses = 0
			if IsRequestReply(s.reqPacket, pdu) {
				s.resetRequestTime()
				debugf("ignore read of reply from the other server")
				continue
			}
			debugf("switch around request and reply pairs")
			s.requestTime = now
			s.reqPacket = pdu
			return n, nil
		}
		return n, errors.New("assert deadcode at end of read")
	}
}

func (s *FailbackSerialConn) Write(b []byte) (int, error) {
	if s.isActive {
		if s.isFailback {
			time.Sleep(SecondaryDelay + s.BytesDelay(len(b)))
			if !s.isActive {
				goto endActive
			}
		}
		s.resetRequestTime()
		return s.serial.Write(b)
	}
endActive:
	debugf("FailbackSerialConn ignore Write:%x\n", b)
	return len(b), nil
}

func (s *FailbackSerialConn) resetRequestTime() {
	s.requestTime = time.Time{}
	s.reqPacket = nil
}

func IsRequestReply(r, a PDU) bool {
	if r.GetFunctionCode() != a.GetFunctionCode() {
		debugf("diff fc\n")
		return false
	}
	if GetPDUSizeFromHeader(r, true) != len(r) {
		debugf("size not req\n")
		return false
	}
	if GetPDUSizeFromHeader(a, false) != len(a) {
		debugf("size not rep\n")
		return false
	}
	eq := false
	switch r.GetFunctionCode() {
	case FcReadCoils, FcReadDiscreteInputs:
		eq = uint8((r.GetRequestCount()+7)/8) == a[1]
	case FcReadHoldingRegisters, FcReadInputRegisters:
		eq = uint8(r.GetRequestCount()*2) == a[1]
	case FcWriteSingleCoil, FcWriteSingleRegister,
		FcWriteMultipleCoils, FcWriteMultipleRegisters:
		eq = bytes.Equal(r[:5], a[:5])
	}
	if !eq {
		debugf("header mismatch\n")
	}
	return eq
}
