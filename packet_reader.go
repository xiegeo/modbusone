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
		read += n
		if err != nil || read == len(p) {
			return read, err
		}
		if n > s.bufferSize {
			debugf("calibrating rtuPacketReader bufferSize to %v", n)
			s.bufferSize = n
		}
		if n < s.bufferSize {
			//buffer is not filled, reader must have detected time based message
			//termination.
			return read, nil
		}
		//Now I have a full buffer read, so I need other ways to detect message
		//termination
		if read < expected {
			//lets read more
			continue
		}
		//lets see if there is more to read
		expected = GetRTUSizeFromHeader(p, s.isClient)
	}
	return read, nil
}

//GetPDUSizeFromHeader returns the expected sized of a pdu packet with the given
//PDU header, if not enough info is in the header, then it returns the shortest possible.
func GetPDUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 2 {
		return 2
	}
	ec, f := FunctionCode(header[0]).SeparateError()
	if ec || !f.Valid() {
		return 2
	}
	if isClient == f.IsReadToServer() {
		return 5
	}
	if isClient {
		if f.IsSingle() {
			return 5
		}
		if len(header) < 6 {
			return 6
		}
		return 6 + int(header[5])
	}
	//server reply to a read
	return 2 + int(header[1])
}

//GetRTUSizeFromHeader returns the expected sized of a rtu packet with the given
//RTU header, if not enough info is in the header, then it returns the shortest possible.
func GetRTUSizeFromHeader(header []byte, isClient bool) int {
	if len(header) < 3 {
		return 3
	}
	return GetPDUSizeFromHeader(header[1:], isClient) + 3
}
