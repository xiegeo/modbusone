package modbusone

import (
	"io"
	"time"
)

type FailbackConn struct {
	conn                     io.ReadWriteCloser
	isServer                 bool
	isFailback               bool
	isActive                 bool
	primaryReactivitionDelay time.Duration //when primary is restarted, how long should it wait to see if there is a failback running.
	primaryForceBackDelay    time.Duration //when a failback is running, how long should it wait to take over again.
	serverMisses             int           //how many misses is the primary server is detected as down
	serverMissesMax          int
	clientMissing            time.Duration //how long untill the primary client is detected as down
	clientLastMessage        time.Time
}
