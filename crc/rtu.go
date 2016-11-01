package crc

//Validate return true if byte slice ends with valid crc
func Validate(bs []byte) bool {
	if len(bs) <= 2 {
		return false
	}
	length := len(bs)
	var c crc
	c.Reset()
	c.Write(bs[:length-2])
	return bs[length-2] == c.low && bs[length-1] == c.high
}

// Sum appends the hash of input to it and returns the resulting slice.
func Sum(bs []byte) []byte {
	return New().Sum(bs)
}
