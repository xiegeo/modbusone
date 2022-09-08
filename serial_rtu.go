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
// over sided implementations encountered in the wild. This setting only applies
// to the server end, since client is always reserved in what it requests.
// Also change OverSizeMaxRTU properly.
var OverSizeSupport = false

// OverSizeMaxRTU overrides MaxRTUSize when OverSizeSupport is true.
var OverSizeMaxRTU = MaxRTUSize

const smallestRTUSize = 4

// RTU is the Modbus RTU Application Data Unit.
type RTU []byte

// MakeRTU makes a RTU with slaveID and PDU.
func MakeRTU(slaveID byte, p PDU) RTU {
	return RTU(crc.Sum(append([]byte{slaveID}, p...)))
}

// IsMulticast returns true if slaveID is the multicast address 0.
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
