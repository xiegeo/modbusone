package modbusone

import (
	"fmt"

	"github.com/xiegeo/modbusone/crc"
)

//Modbus RTU Application Data Unit
type RTU []byte

//MakeRTU makes a RTU with slaveID and PDU
func MakeRTU(slaveID byte, p PDU) RTU {
	return RTU(crc.Append(append([]byte{slaveID}, p...)))
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

//GetPDU returns the PDU inside, with no safty checks.
func (r RTU) fastGetPDU() PDU {
	return PDU(r[1 : len(r)-2])
}
