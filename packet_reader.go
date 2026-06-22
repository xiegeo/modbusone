package modbusone

import (
	"io"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/xiegeo/modbusone/crc"
)

var _ = rand.Int63n

// PacketReader signals that this reader returns full ADU packets.
type PacketReader interface {
	io.Reader
	PacketReaderFace()
}

type rtuPacketReader struct {
	r             SerialContext // the underlining reader
	isClient      bool
	slaveID       byte
	bidirectional bool
	twoWire       bool
	last          []byte
	lastRTU       RTU
	lastReadAt    time.Time
}

// NewRTUPacketReader create a Reader that attempt to read full packets.
// Please use NewRTUPacketReader2 when serving on a 2 wire multi-server connection.
func NewRTUPacketReader(r SerialContext, isClient bool) PacketReader {
	return &rtuPacketReader{r: r, isClient: isClient}
}

// NewRTUPacketReader2 create a Reader that attempt to read full packets.
// Set twoWire to true for 2 wire and false for 4 wire.
func NewRTUPacketReader2(r SerialContext, isClient bool, slaveID byte, twoWire bool) PacketReader {
	return &rtuPacketReader{r: r, isClient: isClient, slaveID: slaveID, twoWire: twoWire}
}

// NewRTUBidirectionalPacketReader create a Reader that attempt to read full packets
// that comes from either server or client.
func NewRTUBidirectionalPacketReader(r SerialContext) PacketReader {
	return &rtuPacketReader{r: r, bidirectional: true}
}

func (s *rtuPacketReader) PacketReaderFace() {}

func (s *rtuPacketReader) Read(p []byte) (int, error) {
	for {
		atomic.AddInt64(&s.r.Stats().ReadPackets, 1)
		expected := smallestRTUSize
		read := 0
		for read < expected {
			if len(s.last) != 0 {
				read += copy(p, s.last)
				s.last = s.last[:0]
			} else {
				// time.Sleep(time.Duration(rand.Int63n(int64(time.Second / 10))))
				n, err := s.r.Read(p[read:])
				if n < 0 { // some users report n = -1 on error
					n = 0
				}
				now := time.Now()
				if read != 0 {
					cutoffDuration := GetPacketCutoffDurationFromSerialContext(s.r, n)
					readDuration := now.Sub(s.lastReadAt)
					if readDuration > cutoffDuration {
						debugf("RTUPacketReader read took:%v > %v, reset packet", readDuration, cutoffDuration)
						s.last = append(s.last[:0], p[read:read+n]...)
						s.lastReadAt = now
						atomic.AddInt64(&s.r.Stats().OtherDrops, 1)
						return read, err
					}
				}
				s.lastReadAt = now
				debugf("RTUPacketReader read (%v+%v)/%v err:%v, expected %v", read, n, len(p), err, expected)
				read += n
				if err != nil || read == len(p) {
					return read, err
				}
			}
			if read < expected {
				// lets read more
				continue
			}
			// lets see if there is more to read
			if s.bidirectional {
				expected = GetRTUBidirectionalSizeFromHeader(p[:read])
				debugf("GetRTUBidirectionalSizeFromHeader new expected size %v %x", expected, p[:read])
			} else if s.twoWire {
				expected = GetRTUSizeFromHeader2(p[:read], s.isClient, s.slaveID, s.lastRTU)
				debugf("GetRTUSizeFromHeader2 new expected size %v %v %x", expected, s.isClient, p[:read])
			} else {
				expected = GetRTUSizeFromHeader(p[:read], s.isClient)
				debugf("GetRTUSizeFromHeader new expected size %v %v %x", expected, s.isClient, p[:read])
			}
			if expected > read-1 { // some devices returns immediately on first byte received, so we let it buffer before calling read again.
				waitForBytes := min(16, expected-read)
				time.Sleep(s.r.BytesDelay(waitForBytes))
			}
		}
		if read > expected {
			if crc.Validate(p[:expected]) {
				atomic.AddInt64(&s.r.Stats().LongReadWarnings, 1)
				s.last = append(s.last[:0], p[expected:read]...)
				debugf("long read warning %v / %v", expected, read)
				s.lastRTU = RTU(p[:expected])
				return expected, nil
			}
		}
		if crc.Validate(p[:read]) {
			s.lastRTU = RTU(p[:read])
			return read, nil
		}
	}
}

// GetPDUSizeFromHeader returns the expected sized of a PDU packet with the given
// PDU header, if not enough info is in the header, then it returns the shortest possible.
// isClient is true if a client/master is reading the packet.
func GetPDUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 2 {
		return 2
	}
	ec, f := FunctionCode(header[0]).SeparateError()
	if ec || !f.Valid() {
		return 2
	}
	if isClient == f.IsWriteToServer() {
		// all packets without data: fc, address, and count
		return 5
	}
	if isClient {
		// all data replies: fc, data bytes, data
		return 2 + int(header[1])
	}
	if f.IsSingle() {
		// fc, address, one data
		return 5
	}
	// fc, address, count, data bytes, data
	if len(header) < 6 {
		return 6
	}
	if OverSizeSupport {
		n := int(header[3])*256 + int(header[4])
		var overSize int
		if f.IsUint16() {
			overSize = 6 + n*2
		} else {
			overSize = 6 + (n-1)/8 + 1
		}
		return min(GetMaxPDUSize(), overSize)
	}
	return 6 + int(header[5])
}

// GetRTUSizeFromHeader returns the expected sized of a RTU packet with the given
// RTU header, if not enough info is in the header, it returns the shortest possible.
// isClient is true if a client/master is reading the packet.
// This function only works properly for 1 to 1 communications.
// Please use GetRTUSizeFromHeader2 for multi slave/server on the same serial port.
func GetRTUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 3 {
		return 3
	}
	if header[0] == 0 {
		return GetPDUSizeFromHeader(header[1:], false) + 3
	}
	return GetPDUSizeFromHeader(header[1:], isClient) + 3
}

// GetRTUSizeFromHeader2 returns the expected sized of a RTU packet with the given
// RTU header, if not enough info is in the header, it returns the shortest needed to disambiguate.
// Under extreme situations, GetRTUSizeFromHeader2 could return a shorter length given more
// data, if the longer but more expected packet interpretation CRC is not correct.
//
// If isClient is false, slaveID is use to check if header data could common from another slave.
//
// If there are multiple slave/server on the same serial port, lastPacket is used to
// disambiguate between requests and replies when necessary.
func GetRTUSizeFromHeader2(header []byte, isClient bool, slaveID byte, lastPacket RTU) int {
	if len(header) < 3 {
		return 3
	}
	packetId := header[0]
	if isClient || packetId == slaveID {
		return GetRTUSizeFromHeader(header[1:], isClient)
	}

	return GetRTUBidirectionalSizeFromHeader2(header, lastPacket)
}

// GetRTUBidirectionalSizeFromHeader is like GetRTUSizeFromHeader, except for any direction
// by checking the CRC for disambiguation of length.
//
// There is currently a slight possibly that a long pack happens to crc correctly to a shorter packet,
// please use GetRTUBidirectionalSizeFromHeader2 for increased safety
func GetRTUBidirectionalSizeFromHeader(header []byte) int {
	s := GetRTUSizeFromHeader(header, false)
	l := GetRTUSizeFromHeader(header, true)
	if s == l {
		return s
	}
	if s > l {
		s, l = l, s
	}
	if s > len(header) {
		return s
	}
	if l <= len(header) && crc.Validate(header[:l]) {
		return l
	}
	if crc.Validate(header[:s]) {
		return s
	}
	return l
}

func GetRTUBidirectionalSizeFromHeader2(header []byte, lastPacket RTU) int {
	if len(lastPacket) == 0 {
		return getRTUBidirectionalSizeFromHeaderWithPrefer(header, false)
	}

	reqLen := GetRTUSizeFromHeader(lastPacket, false)
	if len(lastPacket) == reqLen {
		return getRTUBidirectionalSizeFromHeaderWithPrefer(header, false)
	}
	return getRTUBidirectionalSizeFromHeaderWithPrefer(header, true)
}

func getRTUBidirectionalSizeFromHeaderWithPrefer(header []byte, isClient bool) int {
	size := GetRTUSizeFromHeader(header, isClient)
	if size < len(header) {
		return size
	}
	if crc.Validate(header[:size]) {
		return size
	}
	size2 := GetRTUSizeFromHeader(header, !isClient)
	if size2 < len(header) {
		return size2
	}
	if crc.Validate(header[:size2]) {
		return size2
	}
	return min(size, size2)
}
