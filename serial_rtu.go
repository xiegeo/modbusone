package modbusone

import (
	"fmt"

	"github.com/xiegeo/modbusone/crc"
)

type RTUServer struct {
	SlaveId byte
}

//Modbus RTU Application Data Unit
type RTU []byte

func (r RTU) GetPDU() (PDU, error) {
	if len(r) < 4 {
		return nil, fmt.Errorf("RTU data too short to produce PDU")
	}
	if !crc.Validate(r) {
		return nil, fmt.Errorf("RTU data crc not valid")
	}
	return PDU(r[1 : len(r)-2]), nil
}
