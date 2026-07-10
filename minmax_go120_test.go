//go:build !go1.21

package modbusone_test

// ordered defines the type constraint for types that support <, <=, >, and >=.
type ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

// min returns the smallest of two or more arguments.
func min[T ordered](x T, y ...T) T {
	result := x
	for _, val := range y {
		if val < result {
			result = val
		}
	}
	return result
}

// max returns the largest of two or more arguments.
func max[T ordered](x T, y ...T) T {
	result := x
	for _, val := range y {
		if val > result {
			result = val
		}
	}
	return result
}
