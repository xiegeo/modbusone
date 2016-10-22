package modbusone

import (
	"testing"
)

func TestExceptionCode(t *testing.T) {
	ec := EcIllegalFunction
	t.Log(ec)
	/*
		var err error
		err = ec
		t.Log(err)*/
}
