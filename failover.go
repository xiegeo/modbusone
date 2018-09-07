package modbusone

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"time"
)

//FailoverSerialConn manages a failover connection, which does failover using
//shared serial bus and shared slaveId. Slaves using other ids on the same
//bus is not supported. If the other side supports multiple slave ids, then
//it is best to implement failover on the other side by call different slaveIds.
type FailoverSerialConn struct {
	SerialContext //base SerialContext
	PacketReader
	isClient   bool //client or server
	isFailover bool //primary or failover
	isActive   bool //use atomic, active or passive
	lock       sync.Mutex

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

	//SecondaryDelay is the delay to use on a secondary to give time for the primary to reply first.
	//Default 0.1 seconds.
	SecondaryDelay time.Duration
	//MissDelay is the delay to use by the primary when passive to detect missed packets by secondary.
	//It must be bigger than SecondaryDelay for primary to detect an active failover.
	//Default 0.2 seconds.
	MissDelay time.Duration

	//how many misses is the primary detected as down
	//default 5
	MissesMax int32
	misses    int32
}

//NewFailoverConn adds failover function to a SerialContext
func NewFailoverConn(sc SerialContext, isFailover, isClient bool) *FailoverSerialConn {
	c := &FailoverSerialConn{
		SerialContext:          sc,
		isClient:               isClient,
		isFailover:             isFailover,
		PrimaryDisconnectDelay: 3 * time.Second,
		PrimaryForceBackDelay:  10 * time.Minute,
		SecondaryDelay:         time.Second / 10,
		MissDelay:              time.Second / 5,
		startTime:              time.Now(),
		MissesMax:              3,
	}
	if isFailover {
		c.MissesMax += 2
	}
	c.PacketReader = NewRTUBidirectionalPacketReader(c.SerialContext)
	return c
}

//BytesDelay implements BytesDelay for SerialContext
func (s *FailoverSerialConn) BytesDelay(n int) time.Duration {
	return s.SerialContext.BytesDelay(n)
}

func (s *FailoverSerialConn) serverRead(b []byte) (int, error) {
	locked := false
	defer func() {
		if locked {
			s.lock.Unlock()
		}
	}()
	for {
		if locked {
			s.lock.Unlock()
			locked = false
		}
		n, err := s.PacketReader.Read(b)
		if err != nil {
			return n, err
		}
		s.lock.Lock()
		locked = true

		if !s.isFailover {
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

		rtu := RTU(b[:n])
		pdu, err := rtu.GetPDU()
		if err != nil {
			debugf("failover serverRead internal GetPDU error : %v", err)
			return n, err //bubbles formate up errors
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
				s.setLastReqTime(pdu, time.Now()) //reset is called on write
				return n, nil
			}
			//yes
			s.isActive = false
			s.misses = 0
			s.resetRequestTime()
			debugf("primary found, going from active to passive")
			continue //throw away and read again

		} else {
			//we are passive here
			now := time.Now()
			if s.requestTime.IsZero() {
				s.setLastReqTime(pdu, now)
				return n, nil
			}
			if now.Sub(s.requestTime) > s.MissDelay+s.BytesDelay(n) {
				s.misses++
				if s.misses > s.MissesMax {
					s.isActive = true
				} else {
					s.setLastReqTime(pdu, now)
				}
				return n, nil
			}

			s.misses = 0
			if IsRequestReply(s.reqPacket.Bytes(), pdu) {
				s.resetRequestTime()
				debugf("ignore read of reply from the other server")
				continue
			}
			debugf("switch around request and reply pairs")
			s.setLastReqTime(pdu, now)
			return n, nil
		}
	}
}

func (s *FailoverSerialConn) IsActive() bool {
	s.lock.Lock()
	a := s.isActive
	s.lock.Unlock()
	return a
}

func (s *FailoverSerialConn) describe() string {
	b := strings.Builder{}
	b.WriteString("FailoverSerialConn")
	if s.isClient {
		b.WriteString(" Client")
	} else {
		b.WriteString(" Server")
	}
	if s.isFailover {
		b.WriteString(" Failover")
	} else {
		b.WriteString(" Primary")
	}
	if s.IsActive() {
		b.WriteString(" Active")
	} else {
		b.WriteString(" Passive")
	}
	return b.String()
}

func (s *FailoverSerialConn) clientRead(b []byte) (int, error) {
	n, err := s.PacketReader.Read(b)
	if err != nil {
		return n, err
	}
	now := time.Now()

	rtu := RTU(b[:n])
	pdu, err := rtu.GetPDU()

	s.lock.Lock()
	defer func() {
		s.misses = 0
		s.lock.Unlock()
	}()

	if err != nil {
		debugf("failover clientRead internal GetPDU error : %v", err)
		return n, err //bubbles formate up errors
	}

	isReply := now.Sub(s.requestTime) < s.MissDelay+s.BytesDelay(n) && IsRequestReply(s.reqPacket.Bytes(), pdu)

	if !isReply {
		debugf("got request from other client")
		s.setLastReqTime(pdu, now)
		if s.isFailover && s.isActive {
			debugf("deactivates failover client")
			s.isActive = false
		}
		return n, nil // give requests so caller can match with replies
	}
	s.resetRequestTime()
	return n, nil
}

//Read reads the serial port
func (s *FailoverSerialConn) Read(b []byte) (int, error) {
	defer func() {
		s.lock.Lock()
		s.lastRead = time.Now()
		s.lock.Unlock()
	}()
	if s.isClient {
		return s.clientRead(b)
	}
	return s.serverRead(b)
}

func (s *FailoverSerialConn) Write(b []byte) (int, error) {
	s.lock.Lock()
	locked := true
	debugf("start write c %v, a %v, f %v\n", s.isClient, s.isActive, s.isFailover)
	defer func() {
		if locked {
			s.lock.Unlock()
		}
	}()
	if s.isClient {
		now := time.Now()
		if !s.isFailover {
			if s.isActive {
				if s.lastRead.Add(s.PrimaryDisconnectDelay).Before(now) {
					debugf("primary was disconnected for too long for write to be safe\n")
					s.isActive = false
				}
			}
			if !s.isActive && s.startTime.Add(s.PrimaryForceBackDelay).Before(now) {
				debugf("active server after PrimaryForceBackDelay passed\n")
				s.isActive = true
				s.startTime = now //push back the next force back
			}
		}

		if !s.isActive {
			if s.misses >= s.MissesMax {
				debugf("activities client with %v misses\n", s.misses)
				s.isActive = true
			} else {
				s.misses++
				debugf("%v misses\n", s.misses)
			}
		}

		if s.isActive {
			s.setLastReqTime(RTU(b).fastGetPDU(), now)
			s.lock.Unlock()
			locked = false
			return s.SerialContext.Write(b)
		}
	} else if s.isActive {
		if s.isFailover {
			s.lock.Unlock()
			locked = false
			//give primary time to react first
			time.Sleep(s.SecondaryDelay + s.BytesDelay(len(b)))
			s.lock.Lock()
			locked = true
			if !s.isActive {
				goto endActive
			}
		}
		s.resetRequestTime()
		s.lock.Unlock()
		locked = false
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

//IsRequestReply test if PDUs are a request reply pair, useful for listening to transactions passively.
func IsRequestReply(r, a PDU) bool {
	match := func() bool {
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
	}()
	debugf("IsRequestReply %x %x %v\n", r, a, match)
	return match
}
