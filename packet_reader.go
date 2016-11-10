package modbusone

import (
	"io"
)

type rtuPacketReader struct {
	r          io.Reader //the underlining reader
	isClient   bool
	bufferSize int
}

//NewRTUPacketReader create a Reader that can read full packets.
//
//The input reader r must only return a read for only two siturations:
//reading stopped as per Modbus timing based packet terminater,
//or buffer is full.
//
//isClient set how RTU is interpeted.
//
//bufferSize is the max read size in bytes of the given reader. When a read reaches
//bufferSize, a formate aware RTU delimiter is used.
func NewRTUPacketReader(r io.Reader, isClient bool, bufferSize int) io.Reader {
	return &rtuPacketReader{r, isClient, bufferSize}
}

func (s *rtuPacketReader) Read(p []byte) (int, error) {
	expected := smallestRTUSize
	read := 0
	for read < expected {
		n, err := s.r.Read(p[read:])
		debugf("RTUPacketReader read (%v+%v)/%v %v, expcted %v, bufferSize %v", read, n, len(p), err, expected, s.bufferSize)
		read += n
		if err != nil || read == len(p) {
			return read, err
		}
		if n > s.bufferSize {
			debugf("recalibrating rtuPacketReader bufferSize to %v", n)
			s.bufferSize = n
		}
		if read > 0 && n < s.bufferSize {
			//some data is read, but buffer is not filled,
			//reader must have detected time based message termination.
			return read, nil
		}
		if read < expected {
			//lets read more
			continue
		}
		//lets see if there is more to read
		expected = GetRTUSizeFromHeader(p, s.isClient)
		debugf("RTUPacketReader new expected size %v", expected)
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
	//fc, adress, count, data bytes, data
	if len(header) < 6 {
		return 6
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
