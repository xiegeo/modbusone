package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/tarm/serial"
	"github.com/xiegeo/modbusone"
)

var _ = time.Second

var (
	address  = flag.String("l", "", "required device location, such as: /dev/ttyS0 in linux or com1 in windows")
	baudRate = flag.Int("r", 19200, "baud rate")
	parity   = flag.String("p", "E", "parity: N - None, E - Even, O - Odd")
	stopBits = flag.Int("s", 1, "stop bits: 1 or 2")

	isClient = flag.Bool("c", false, "true for client, false (default) for server. The client is interactive.")
	slaveID  = flag.Uint64("id", 1, "the slaveId of the server for serial communication, 0 for multicast only")
	fillData = flag.String("d", "am3", "data to start with, am3 starts memory "+
		"with bools as address (mod 3) == 0, and registers as address * 3 (mod uint16)")

	writeSizeLimit = flag.Int("wsl", modbusone.MaxRTUSize, "client only, the max size in bytes of a write to server to send")
	readSizeLimit  = flag.Int("rsl", modbusone.MaxRTUSize, "client only, the max size in bytes of a read from server to request")

	verbose = flag.Bool("v", false, "prints debugging information")
)

func main() {
	flag.Parse()
	if *verbose {
		modbusone.SetDebugOut(os.Stdout)
	}
	config := serial.Config{
		Name:     *address,
		Baud:     *baudRate,
		StopBits: serial.StopBits(*stopBits),
	}
	if len(*parity) > 1 {
		config.Parity = []serial.Parity(*parity)[0]
	}
	s, err := serial.OpenPort(&config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open serial error: %v\n", err)
		os.Exit(1)
	}
	com := modbusone.NewSerialContext(s, int64(*baudRate))
	defer func() {
		fmt.Printf("%+v\n", com.Stats())
		com.Close()
	}()
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		fmt.Printf("%+v\n", com.Stats())
		fmt.Println("close serial port")
		com.Close()
		os.Exit(0)
	}()

	id, err := modbusone.Uint64ToSlaveID(*slaveID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "set slaveID error: %v\n", err)
		os.Exit(1)
	}
	if *fillData == "am3" {
		fillAm3()
	}
	var device modbusone.ServerCloser
	if *isClient {
		if *writeSizeLimit > modbusone.MaxRTUSize || *readSizeLimit > modbusone.MaxRTUSize {
			fmt.Fprintf(os.Stderr, "write/read size limit is too big")
			os.Exit(1)
		}
		client := modbusone.NewRTUClient(com, id)
		go runClient(client)
		device = client
	} else {
		device = modbusone.NewRTUServer(com, id)
	}
	h := modbusone.SimpleHandler{
		ReadDiscreteInputs: func(address, quantity uint16) ([]bool, error) {
			fmt.Printf("ReadDiscreteInputs from %v, quantity %v\n", address, quantity)
			return discretes[address : address+quantity], nil
		},
		WriteDiscreteInputs: func(address uint16, values []bool) error {
			fmt.Printf("WriteDiscreteInputs from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				discretes[address+uint16(i)] = v
			}
			return nil
		},

		ReadCoils: func(address, quantity uint16) ([]bool, error) {
			fmt.Printf("ReadCoils from %v, quantity %v\n", address, quantity)
			return coils[address : address+quantity], nil
		},
		WriteCoils: func(address uint16, values []bool) error {
			fmt.Printf("WriteCoils from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				coils[address+uint16(i)] = v
			}
			return nil
		},

		ReadInputRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("ReadInputRegisters from %v, quantity %v\n", address, quantity)
			return inputRegisters[address : address+quantity], nil
		},
		WriteInputRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("WriteInputRegisters from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				inputRegisters[address+uint16(i)] = v
			}
			return nil
		},

		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("ReadHoldingRegisters from %v, quantity %v\n", address, quantity)
			return holdingRegisters[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("WriteHoldingRegisters from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				holdingRegisters[address+uint16(i)] = v
			}
			return nil
		},

		OnErrorImp: func(req modbusone.PDU, errRep modbusone.PDU) {
			fmt.Printf("error received: %v from req: %v\n", errRep, req)
		},
	}
	err = device.Serve(&h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
		os.Exit(1)
	}
}

const size = 0x10000

var (
	discretes        [size]bool
	coils            [size]bool
	inputRegisters   [size]uint16
	holdingRegisters [size]uint16
)

func fillAm3() {
	for i := range discretes {
		discretes[i] = i%3 == 0
	}
	for i := range coils {
		coils[i] = i%3 != 0
	}
	for i := range inputRegisters {
		inputRegisters[i] = uint16(i * 3)
	}
	for i := range holdingRegisters {
		holdingRegisters[i] = uint16(0xFFFF - i)
	}
}
