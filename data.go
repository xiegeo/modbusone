package modbusone

func DataToBools(data []byte, count uint16, fc FunctionCode) ([]bool, error) {
	if fc == FcWriteSingleCoil {
		if len(data) != 2 {
			return nil, EcIllegalDataValue
		}
		if data[1] != 0 {
			return nil, EcIllegalDataValue
		}
		if data[0] == 0 {
			return []bool{false}, nil
		}
		if data[0] == 0xff {
			return []bool{true}, nil
		}
		return nil, EcIllegalDataValue
	}

	byteCount := len(data)
	if (count+7)/8 != uint16(byteCount) {
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
