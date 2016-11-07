package modbusone

import (
	"fmt"

	"github.com/xiegeo/modbusone/crc"
)

//MaxRTUSize is the max possible size of a RTU packet
const MaxRTUSize = 256

//MaxRTUSize is the max possible size of a PDU packet
const MaxPDUSize = 253

const smallestRTUSize = 4

//StartingSerialBufferSide is the default buffer size to pass to NewRTUPacketReader
var StartingSerialBufferSide = 32

//RTU is the Modbus RTU Application Data Unit
type RTU []byte

//MakeRTU makes a RTU with slaveID and PDU
func MakeRTU(slaveID byte, p PDU) RTU {
	return RTU(crc.Sum(append([]byte{slaveID}, p...)))
}

//IsMulticast returns true if slaveID is the multicast address 0
func (r RTU) IsMulticast() bool {
	return len(r) > 0 && r[0] == 0
}

//GetPDU returns the PDU inside, CRC is checked.
func (r RTU) GetPDU() (PDU, error) {
	if len(r) < 4 {
		return nil, fmt.Errorf("RTU data too short to produce PDU")
	}
	if !crc.Validate(r) {
		return nil, fmt.Errorf("RTU data crc not valid")
	}
	p := r.fastGetPDU()
	return p, nil
}

//GetPDU returns the PDU inside, with no safety checks.
func (r RTU) fastGetPDU() PDU {
	return PDU(r[1 : len(r)-2])
}
