package modbusone

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestToExceptionCode(t *testing.T) {
	tests := []struct {
		err  error
		want ExceptionCode
	}{
		{err: EcIllegalDataAddress, want: EcIllegalDataAddress},
		{err: ErrFcNotSupported, want: EcIllegalFunction},
		{err: errors.New("other errors"), want: EcServerDeviceFailure},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := ToExceptionCode(tt.err); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToExceptionCode() = %v, want %v", got, tt.want)
			}
			errWarped := fmt.Errorf("warped error for %w", tt.err)
			if got := ToExceptionCode(errWarped); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToExceptionCode() = %v, want %v", got, tt.want)
			}
		})
	}
}
