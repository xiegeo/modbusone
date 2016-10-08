package crc

func Validate(bs []byte) bool {
	if len(bs) <= 2 {
		return false
	}
	length := len(bs)
	var crc crc
	crc.Reset()
	crc.Write(bs[0 : length-2])
	return bs[length-2] == crc.low && bs[length-1] == crc.high
}

func Append(bs []byte) []byte {
	return New().Sum(bs)
}
