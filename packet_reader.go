package modbusone

import (
	"io"
	"math/rand"
	"time"

	"github.com/xiegeo/modbusone/crc"
)

var _ = rand.Int63n

//PacketReader signals that this reader returns full ADU packets
type PacketReader interface {
	io.Reader
	PacketReaderFace()
}

type rtuPacketReader struct {
	r           SerialContext //the underlining reader
	isClient    bool
	bidirection bool
	last        []byte
}

//NewRTUPacketReader create a Reader that attempt to read full packets.
func NewRTUPacketReader(r SerialContext, isClient bool) PacketReader {
	return &rtuPacketReader{r, isClient, false, nil}
}

//NewRTUBidirectionalPacketReader create a Reader that attempt to read full packets
//that comes from either server or client.
func NewRTUBidirectionalPacketReader(r SerialContext) PacketReader {
	return &rtuPacketReader{r, false, true, nil}
}

func (s *rtuPacketReader) PacketReaderFace() {}

func (s *rtuPacketReader) Read(p []byte) (int, error) {
	s.r.Stats().ReadPackets++
	expected := smallestRTUSize
	read := 0
	for read < expected {
		if len(s.last) != 0 {
			read += copy(p, s.last)
			s.last = s.last[:0]
		} else {
			//time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
			n, err := s.r.Read(p[read:])
			debugf("RTUPacketReader read (%v+%v)/%v %v, expcted %v", read, n, len(p), err, expected)
			read += n
			if err != nil || read == len(p) {
				return read, err
			}
		}
		if read < expected {
			//lets read more
			continue
		}
		//lets see if there is more to read
		if s.bidirection {
			expected = GetRTUBidirectionSizeFromHeader(p[:read])
			debugf("RTUPacketReader new expected size %v %x", expected, p[:read])
		} else {
			expected = GetRTUSizeFromHeader(p[:read], s.isClient)
			debugf("RTUPacketReader new expected size %v %v %x", expected, s.isClient, p[:read])
		}
		if expected > read-1 {
			time.Sleep(s.r.BytesDelay(expected - read))
		}
	}
	if read > expected {
		if crc.Validate(p[:expected]) {
			s.r.Stats().LongReadWarnings++
			s.last = append(s.last[:0], p[expected:read]...)
			debugf("long read warning %v / %v", expected, read)
			return expected, nil
		}
		if crc.Validate(p[:read]) {
			s.r.Stats().FormateWarnings++
		}
	}
	return read, nil
}

//GetPDUSizeFromHeader returns the expected sized of a pdu packet with the given
//PDU header, if not enough info is in the header, then it returns the shortest possible.
//isClient is true if a client/master is reading the packet.
func GetPDUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 2 {
		return 2
	}
	ec, f := FunctionCode(header[0]).SeparateError()
	if ec || !f.Valid() {
		return 2
	}
	if isClient == f.IsWriteToServer() {
		//all packets without data: fc, address, and count
		return 5
	}
	if isClient {
		//all data replies: fc, data bytes, data
		return 2 + int(header[1])
	}
	if f.IsSingle() {
		//fc, address, one data
		return 5
	}
	//fc, address, count, data bytes, data
	if len(header) < 6 {
		return 6
	}
	if OverSizeSupport {
		n := int(header[3])*256 + int(header[4])
		if f.IsUint16() {
			return 6 + n*2
		}
		return 6 + (n-1)/8 + 1
	}
	return 6 + int(header[5])
}

//GetRTUSizeFromHeader returns the expected sized of a rtu packet with the given
//RTU header, if not enough info is in the header, then it returns the shortest possible.
//isClient is true if a client/master is reading the packet.
func GetRTUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 3 {
		return 3
	}
	return GetPDUSizeFromHeader(header[1:], isClient) + 3
}

//GetRTUBidirectionSizeFromHeader is like GetRTUSizeFromHeader, except for any direction
//by checking the hash for disambiguation.
func GetRTUBidirectionSizeFromHeader(header []byte) int {
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
