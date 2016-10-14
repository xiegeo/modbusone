package modbusone

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestRTU(t *testing.T) {
	testCases := []RTU{
		//from http://www.simplymodbus.ca/FC16.htm
		RTU([]byte{0x11, 0x10, 0x00, 0x01, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02, 0xC6, 0xF0}),
		RTU([]byte{0x11, 0x10, 0x00, 0x01, 0x00, 0x02, 0x12, 0x98}),
	}

	failCases := []RTU{
		RTU([]byte{}),                                               //too short
		RTU([]byte{0x02, 0x12, 0x98}),                               //too short
		RTU([]byte{0xf1, 0x10, 0x00, 0x01, 0x00, 0x02, 0x12, 0x98}), //crc error
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%v:%v", i, hex.EncodeToString(tc)), func(t *testing.T) {
			p, err := tc.GetPDU()
			if err != nil {
				t.Fatal(err)
			}
			t.Log("PDU:", hex.EncodeToString(p))
			ec := p.Validate()
			if ec != EcOK {
				t.Fatal("error code", ec)
			}
		})
	}

	for i, tc := range failCases {
		t.Run(fmt.Sprintf("ShouldFail%v:%v", i, hex.EncodeToString(tc)), func(t *testing.T) {
			_, err := tc.GetPDU()
			if err == nil {
				t.Fatal("Expected error here")
			}
			t.Log("expected err:", err)
		})
	}
}

func TestRTUDelay(t *testing.T) {
	testCases := []struct {
		bw    int
		delay time.Duration
	}{
		{1, 35 * 11 * time.Second / 10},
		{19200, 2005209},
		{38400, 1750000},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Delay for bw:%v", tc.bw), func(t *testing.T) {
			d := RTUMinDelay(tc.bw)
			if d != tc.delay {
				t.Fatalf("expected:%v got:%v", tc.delay, d)
			}
		})
	}
}
