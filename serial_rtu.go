package modbusone

import (
	"fmt"

	"github.com/xiegeo/modbusone/crc"
)

// MaxRTUSize is the max possible size of a RTU packet.
const MaxRTUSize = 256

// MaxPDUSize is the max possible size of a PDU packet.
const MaxPDUSize = 253

// OverSizeSupport ignores max packet size and encoded number of bytes to support
// over sized implementations encountered in the wild. This setting only applies
// to the server end, since client is always reserved in what it requests.
// Also change OverSizeMaxRTU properly.
var OverSizeSupport = false

// OverSizeMaxRTU overrides MaxRTUSize when OverSizeSupport is true.
// It should not be smaller than MaxRTUSize.
var OverSizeMaxRTU = MaxRTUSize

func GetMaxPDUSize() int {
	if OverSizeSupport {
		return max(MaxPDUSize, OverSizeMaxRTU-3)
	}
	return MaxPDUSize
}

// A prefix of the RTU
type RTUHeader struct {
	SlaveID byte
	PDU
}

const smallestRTUSize = 4

// RTU is the Modbus RTU Application Data Unit.
type RTU []byte

// MakeRTU makes a RTU with slaveID and PDU.
func MakeRTU(slaveID byte, p PDU) RTU {
	return RTU(crc.Sum(append([]byte{slaveID}, p...)))
}

func (r RTU) fastGetHeader() RTUHeader {
	return RTUHeader{
		SlaveID: r.GetSlaveID(),
		PDU:     r.fastGetPDU(),
	}
}

// GetSlaveID returns the SlaveID inside, or 255 if the RTU is empty.
func (r RTU) GetSlaveID() byte {
	if len(r) == 0 {
		return 255 // invalid SlaveID
	}
	return r[0]
}

// IsMulticast returns true if SlaveID is the multicast address 0.
func (r RTU) IsMulticast() bool {
	return len(r) > 0 && r[0] == 0
}

// ErrorCrc indicates data corruption detected by checking the CRC.
var ErrorCrc = fmt.Errorf("RTU data crc not valid")

// GetPDU returns the PDU inside, CRC is checked.
func (r RTU) GetPDU() (PDU, error) {
	if len(r) < 4 {
		return nil, fmt.Errorf("RTU data too short to produce PDU")
	}
	if !crc.Validate(r) {
		return nil, ErrorCrc
	}
	p := r.fastGetPDU()
	return p, nil
}

// fastGetPDU returns the PDU inside, with no safety checks.
func (r RTU) fastGetPDU() PDU {
	return PDU(r[1 : len(r)-2])
}
