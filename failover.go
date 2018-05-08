package modbusone

import (
	"bytes"
	"errors"
	"time"
)

var tnow = time.Now //allows for testing with fake time

var SecondaryDelay = time.Second / 10
var MissDelay = time.Second / 5 //must be bigger than SecondaryDelay for primary to detect an active failover

//FailoverSerialConn manages a failover connection, which does failover using
//shared serial bus and shared slaveId. Slaves using other ids on the same
//bus is not supported. If the other side supports multiple slave ids, then
//it is best to implement failover on the other side by call different slaveIds.
type FailoverSerialConn struct {
	SerialContext //base SerialContext
	PacketReader
	isServer   bool //client or server
	isFailover bool //primary or failover
	isActive   bool //active or passive

	requestTime time.Time //time of the last packet observed passively
	reqPacket   bytes.Buffer
	lastRead    time.Time

	//if primary has not received data for this long, it thinks it's disconnected
	//and go passive, just like at restart
	//default 10 seconds
	PrimaryDisconnectDelay time.Duration

	//when a failover is running,
	//how long should it wait to take over again.
	//default 10 mins
	PrimaryForceBackDelay time.Duration
	startTime             time.Time

	//how many misses is the primary server is detected as down
	//default 5
	ServerMissesMax int
	serverMisses    int

	//how long until the primary client is detected as down
	ClientMissing     time.Duration
	clientLastMessage time.Time
}

//NewFailoverConn adds failover function to a SerialContext
func NewFailoverConn(sc SerialContext, isFailover, isServer bool) *FailoverSerialConn {
	c := &FailoverSerialConn{
		SerialContext:          sc,
		isServer:               isServer,
		isFailover:             isFailover,
		PrimaryDisconnectDelay: 3 * time.Second,
		PrimaryForceBackDelay:  10 * time.Minute,
		startTime:              tnow(),
		ServerMissesMax:        3,
	}
	if isFailover {
		c.ServerMissesMax += 2
	}
	c.PacketReader = NewRTUBidirectionalPacketReader(c.SerialContext)
	return c
}

func (s *FailoverSerialConn) serverRead(b []byte) (int, error) {
	for {
		n, err := s.PacketReader.Read(b)
		if err != nil {
			return n, err
		}
		if !s.isFailover {
			if !s.isActive {
				if s.startTime.Add(s.PrimaryForceBackDelay).Before(tnow()) {
					debugf("force active of primary/n")
					s.isActive = true
				}
			}
			if s.isActive {
				if s.lastRead.Add(s.PrimaryDisconnectDelay).Before(tnow()) {
					debugf("primary was disconnected for too long/n")
					s.isActive = false
					s.startTime = tnow()
				} else {
					return n, nil
				}
			}
		}

		rtu := RTU(b[:n])
		pdu, err := rtu.GetPDU()
		if err != nil {
			debugf("failover internal GetPDU error : %v", err)
			return n, nil //bubbles formate up errors
		}
		if rtu[0] == 0 {
			//zero slave id do not have a reply, so we won't expect one
			s.resetRequestTime()
			return n, nil
		}
		if s.isActive {
			if !s.isFailover {
				return 0, errors.New("assert isFailover")
			}
			//are we getting interrupted?
			if s.requestTime.IsZero() {
				//this should be a client request
				s.setLastReqTime(pdu, tnow()) //reset is called on write
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
			now := tnow()
			if s.requestTime.IsZero() {
				s.setLastReqTime(pdu, now)
				return n, nil
			}
			if now.Sub(s.requestTime) > MissDelay+s.BytesDelay(n) {
				s.serverMisses++
				if s.serverMisses > s.ServerMissesMax {
					s.isActive = true
				} else {
					s.setLastReqTime(pdu, now)
				}
				return n, nil
			}

			s.serverMisses = 0
			if IsRequestReply(s.reqPacket.Bytes(), pdu) {
				s.resetRequestTime()
				debugf("ignore read of reply from the other server")
				continue
			}
			debugf("switch around request and reply pairs")
			s.setLastReqTime(pdu, now)
			return n, nil
		}
		return n, errors.New("assert deadcode at end of read")
	}
}

func (s *FailoverSerialConn) clientRead(b []byte) (int, error) {
	for {
		n, err := s.PacketReader.Read(b)
		if err != nil {
			return n, err
		}

	}
}

//Read reads the serial port
func (s *FailoverSerialConn) Read(b []byte) (int, error) {
	defer func() {
		s.lastRead = tnow()
	}()
	if s.isServer {
		return s.serverRead(b)
	}
	return s.clientRead(b)
}

func (s *FailoverSerialConn) Write(b []byte) (int, error) {
	if s.isActive {
		if s.isFailover {
			time.Sleep(SecondaryDelay + s.BytesDelay(len(b)))
			if !s.isActive {
				goto endActive
			}
		}
		s.resetRequestTime()
		return s.SerialContext.Write(b)
	}
endActive:
	debugf("FailoverSerialConn ignore Write:%x\n", b)
	return len(b), nil
}

func (s *FailoverSerialConn) resetRequestTime() {
	s.requestTime = time.Time{} //zero time
	s.reqPacket.Reset()
}

func (s *FailoverSerialConn) setLastReqTime(pdu PDU, now time.Time) {
	s.requestTime = now
	s.reqPacket.Reset()
	s.reqPacket.Write(pdu)
}

//IsRequestReply test if PDUs are a request reply pair, useful for lessening to transactions passively.
func IsRequestReply(r, a PDU) bool {
	debugf("IsRequestReply %x %x\n", r, a)
	if r.GetFunctionCode() != a.GetFunctionCode() {
		debugf("diff fc\n")
		return false
	}
	if GetPDUSizeFromHeader(r, false) != len(r) {
		debugf("r size not req %v, %x\n", GetPDUSizeFromHeader(r, true), r)
		return false
	}
	if GetPDUSizeFromHeader(a, true) != len(a) {
		debugf("a size not rep %v, %x\n", GetPDUSizeFromHeader(a, false), a)
		return false
	}
	c, err := r.GetRequestCount()
	if err != nil {
		debugf("GetRequestCount error %v\n", err)
		return false
	}
	eq := false
	switch r.GetFunctionCode() {
	case FcReadCoils, FcReadDiscreteInputs:
		eq = uint8((c+7)/8) == a[1]
	case FcReadHoldingRegisters, FcReadInputRegisters:
		eq = uint8(c*2) == a[1]
	case FcWriteSingleCoil, FcWriteSingleRegister,
		FcWriteMultipleCoils, FcWriteMultipleRegisters:
		eq = bytes.Equal(r[:5], a[:5])
	}
	if !eq {
		debugf("header mismatch\n")
	}
	return eq
}
