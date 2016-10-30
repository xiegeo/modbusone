package modbusone

import (
	"encoding/binary"
	"fmt"
)

//DataToBools translates the data part of PDU to []bool dependent on FunctionCode.
func DataToBools(data []byte, count uint16, fc FunctionCode) ([]bool, error) {
	if fc == FcWriteSingleCoil {
		if len(data) != 2 {
			debugf("WriteSingleCoil need 2 bytes data\n")
			return nil, EcIllegalDataValue
		}
		if data[1] != 0 {
			debugf("WriteSingleCoil unexpected %v %v\n", data[0], data[1])
			return nil, EcIllegalDataValue
		}
		if data[0] == 0 {
			return []bool{false}, nil
		}
		if data[0] == 0xff {
			return []bool{true}, nil
		}
		debugf("WriteSingleCoil unexpected %v %v", data[0], data[1])
		return nil, EcIllegalDataValue
	}

	byteCount := len(data)
	if (count+7)/8 != uint16(byteCount) {
		debugf("unexpected size: bools %v, bytes %v", count, byteCount)
		return nil, EcIllegalDataValue
	}
	r := make([]bool, byteCount*8)
	for i := 0; i < byteCount; i++ {
		for j := 0; j < 8; j++ {
			r[i*8+j] = bool((int(data[i]) & (1 << uint(j))) > 0)
		}
	}
	return r[:count], nil
}

//BoolsToData translates []bool to the data part of PDU dependent on FunctionCode.
func BoolsToData(values []bool, fc FunctionCode) ([]byte, error) {
	if fc == FcWriteSingleCoil {
		if len(values) != 1 {
			return nil, fmt.Errorf("FcWriteSingleCoil can not write %v coils", len(values))
		}
		if values[0] {
			return []byte{0xff, 0x00}, nil
		}
		return []byte{0x00, 0x00}, nil
	}

	count := len(values)
	byteCount := (count + 7) / 8
	data := make([]byte, byteCount)

	byteNr := 0
	bitNr := uint8(0)
	byteVal := uint8(0)

	for v := 0; v < count; v++ {
		if values[v] {
			byteVal |= 1 << bitNr
		}
		if bitNr == 7 {
			data[byteNr] = byteVal
			byteVal = 0
			bitNr = 0
			byteNr++
		} else if v+1 == count {
			data[byteNr] = byteVal
		} else {
			bitNr++
		}
	}

	return data, nil
}

//DataToRegisters translates the data part of PDU to []uint16.
func DataToRegisters(data []byte) ([]uint16, error) {
	if len(data) < 2 || len(data)%2 != 0 {
		debugf("unexpected odd number of bytes %v", len(data))
		return nil, EcIllegalDataValue
	}
	count := len(data) / 2
	values := make([]uint16, count)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(data[2*i:])
	}
	return values, nil
}

//RegistersToData translates []uint16 to the data part of PDU.
func RegistersToData(values []uint16) ([]byte, error) {
	data := make([]byte, 2*len(values))
	for i, v := range values {
		binary.BigEndian.PutUint16(data[i*2:], v)
	}
	return data, nil
}
