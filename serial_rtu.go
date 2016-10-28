package modbusone

import (
	"fmt"

	"github.com/xiegeo/modbusone/crc"
)

//Modbus RTU Application Data Unit
type RTU []byte

func MakeRTU(slaveId byte, p PDU) RTU {
	return RTU(crc.Append(append([]byte{slaveId}, p...)))
}

func (r RTU) IsMulticast() bool {
	return len(r) > 0 && r[0] == 0
}

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

func (r RTU) fastGetPDU() PDU {
	return PDU(r[1 : len(r)-2])
}
