package modbusone

import (
	"io"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xiegeo/modbusone/crc"
)

type mockReadOnlySerial struct {
	io.Reader
	io.WriteCloser // leave as nil since not used
}

func FuzzReaderDrop(f *testing.F) {

	cpuHiccup := 1 * time.Millisecond
	sleepDuration := 2 * time.Millisecond

	f.Fuzz(func(t *testing.T,
		isClient bool,
		slaveID byte,
		bidirectional bool,
		TwoWire bool,
		SleepBufferBytes uint8,
		readerBuffer uint16,

		sleep1 bool, data1 []byte, crc1 bool,
		sleep2 bool, data2 []byte, crc2 bool,
		sleep3 bool, data3 []byte, crc3 bool,
	) {
		if slaveID == 0 {
			return // invalid configuration
		}

		pr, pw := io.Pipe()

		o := Option{
			CPUHiccup:        cpuHiccup,
			TwoWire:          TwoWire,
			SleepBufferBytes: int(SleepBufferBytes),
		}
		r := &rtuPacketReader{
			r: &serial{
				conn:     mockReadOnlySerial{Reader: pr},
				baudRate: math.MaxInt64,
				Option:   o,
			},
			isClient:      isClient,
			slaveID:       slaveID,
			bidirectional: bidirectional,
			option:        o,
		}

		finalData := crc.Sum([]byte{slaveID, 0x01, 0x00, 0x13, 0x00, 0x25}) // request data
		if isClient {
			finalData = crc.Sum([]byte{slaveID, 0x01, 0x05, 0xCD, 0x6B, 0xB2, 0x0E, 0x1B}) // resp date
		}

		setup := atomic.Bool{}
		setup.Store(true) // Start step 1: use fuzz to set the packet reader into all possible states.
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, int(readerBuffer)+len(finalData))
			var n int
			var err error
			for setup.Load() {
				n, err = r.Read(buf)
			}
			assert.NoError(t, err)
			assert.Equal(t, finalData, buf[:n])
		}()

		fuzzData := []struct {
			sleep bool
			data  []byte
			crc   bool
		}{{sleep1, data1, crc1}, {sleep2, data2, crc2}, {sleep3, data3, crc3}}
		for _, fd := range fuzzData {
			if fd.sleep {
				time.Sleep(sleepDuration)
			}
			data := fd.data
			if fd.crc {
				data = crc.Sum(data)
			}
			n, err := pw.Write(data)
			assert.Equal(t, len(data), n)
			assert.NoError(t, err)
		}
		time.Sleep(sleepDuration) // long enough duration to drop all unfinished data
		setup.Store(false)        // End step 1

		n, err := pw.Write(finalData)
		assert.Equal(t, len(finalData), n)
		assert.NoError(t, err)

		wg.Wait()
	})

}
