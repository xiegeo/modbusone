package modbusone

import (
	"fmt"
	"time"

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

//RTUMinDelay returns the minum Delay of 3.5 chars between packets or 1750 mircos
func RTUMinDelay(baudRate int) time.Duration {
	delay := 1750 * time.Microsecond
	br := time.Duration(baudRate)
	if br <= 19200 {
		//time it takes to send 3.5 or 7/2 chars (a char of 8 bits takes 11 bits on wire)
		delay = (time.Second*11*7 + (br * 2) - 1) / (br * 2)
	}
	return delay
}

//RTUMinDelay returns the time it takes to send chars in baudRate
func RTUBytesDelay(chars, baudRate int) time.Duration {
	br := time.Duration(baudRate)
	cs := time.Duration(chars)

	return (time.Second*11*cs + br - 1) / br
}
