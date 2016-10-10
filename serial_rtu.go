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
	p := PDU(r[1 : len(r)-2])
	return p, nil
}

//the minum Delay of 3.5 chars between packets
func RTUMinDelay(s SerialPort) time.Duration {
	delay := 1750 * time.Microsecond
	br := time.Duration(s.BaudRate)
	if br <= 19200 {
		oneBitDuration := (time.Second + br - 1) / br //time it takes to send a bit
		delay = (oneBitDuration*11*7 + 1) / 2         //time it takes to send 3.5 chars (a char of 8 bits takes 11 bits on wire)
	}
	return delay
}
