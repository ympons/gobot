// Package client provies a client for interacting with microcontrollers
// using the Firmata protocol https://github.com/firmata/protocol.
package client

import (
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// Pin Modes
const (
	Input  = 0x00
	Output = 0x01
	Analog = 0x02
	Pwm    = 0x03
	Servo  = 0x04
)

// Sysex Codes
const (
	ProtocolVersion          byte = 0xF9
	SystemReset              byte = 0xFF
	DigitalMessage           byte = 0x90
	DigitalMessageRangeStart byte = 0x90
	DigitalMessageRangeEnd   byte = 0x9F
	AnalogMessage            byte = 0xE0
	AnalogMessageRangeStart  byte = 0xE0
	AnalogMessageRangeEnd    byte = 0xEF
	ReportAnalog             byte = 0xC0
	ReportDigital            byte = 0xD0
	PinMode                  byte = 0xF4
	StartSysex               byte = 0xF0
	EndSysex                 byte = 0xF7
	CapabilityQuery          byte = 0x6B
	CapabilityResponse       byte = 0x6C
	PinStateQuery            byte = 0x6D
	PinStateResponse         byte = 0x6E
	AnalogMappingQuery       byte = 0x69
	AnalogMappingResponse    byte = 0x6A
	StringData               byte = 0x71
	I2CRequest               byte = 0x76
	I2CReply                 byte = 0x77
	I2CConfig                byte = 0x78
	FirmwareQuery            byte = 0x79
	I2CModeWrite             byte = 0x00
	I2CModeRead              byte = 0x01
	I2CmodeContinuousRead    byte = 0x02
	I2CModeStopReading       byte = 0x03
	ServoConfig              byte = 0x70
)

// Errors
var (
	ErrConnected = errors.New("client is already connected")
)

// Client represents a client connection to a firmata board
type Client struct {
	pins             []Pin
	firmwareName     string
	protocolVersion  string
	connected        bool
	connection       io.ReadWriteCloser
	analogPins       []int
	initTimeInterval time.Duration
	pinReportChan    map[int]chan Pin
	pinStateChan     map[int]chan Pin
	i2cChan          chan I2cReply
	boolChan         map[string]chan bool
	stringDataChan   chan string
	Error            chan error
}

// Pin represents a pin on the firmata board
type Pin struct {
	SupportedModes []int
	Mode           int
	Value          int
	State          int
	AnalogChannel  int
}

// I2cReply represents the response from an I2cReply message
type I2cReply struct {
	Address  int
	Register int
	Data     []byte
}

// New returns a new Client
func New() *Client {
	c := &Client{
		protocolVersion: "",
		firmwareName:    "",
		connection:      nil,
		pins:            []Pin{},
		analogPins:      []int{},
		connected:       false,
		pinReportChan:   make(map[int]chan Pin),
		pinStateChan:    make(map[int]chan Pin),
		i2cChan:         make(chan I2cReply),
		stringDataChan:  make(chan string),
		boolChan: map[string]chan bool{
			"CapabilityQuery":    make(chan bool),
			"AnalogMappingQuery": make(chan bool),
			"FirmwareQuery":      make(chan bool),
			"ProtocolVersion":    make(chan bool),
		},
	}

	return c
}

// Disconnect disconnects the Client
func (b *Client) Disconnect() (err error) {
	b.connected = false
	return b.connection.Close()
}

// Connected returns the current connection state of the Client
func (b *Client) Connected() bool {
	return b.connected
}

// FirmwareName returns the name of the current firmware
func (b *Client) FirmwareName() string {
	return b.firmwareName
}

// ProtocolVersion returns the current firmata version
func (b *Client) ProtocolVersion() string {
	return b.protocolVersion
}

// Pins returns all available pins
func (b *Client) Pins() []Pin {
	return b.pins
}

// Connect connects to the Client given conn. It first resets the firmata board
// then continuously polls the firmata board for new information when it's
// available.
func (c *Client) Connect(conn io.ReadWriteCloser) (err error) {
	if c.connected {
		return ErrConnected
	}

	c.connection = conn
	if err := c.Reset(); err != nil {
		return err
	}

	go func() {
		s, _ := c.ProtocolVersionQuery()
		<-s
		s, _ = c.FirmwareQuery()
		<-s
		s, _ = c.CapabilityQuery()
		<-s
		s, _ = c.AnalogMappingQuery()
		c.connected = true
		<-s
		c.ReportDigital(0, 1)
		c.ReportDigital(1, 1)
	}()

	for {
		if err := c.process(); err != nil {
			return err
		}
		if c.connected {
			go func() {
				for {
					if !c.connected {
						break
					}
					if err := c.process(); err != nil {
						select {
						case c.Error <- err:
						default:
						}
					}
				}
			}()
			break
		}
	}
	return
}

// Reset sends the SystemReset sysex code.
func (b *Client) Reset() error {
	return b.write([]byte{SystemReset})
}

// SetPinMode sets the pin to mode.
func (b *Client) SetPinMode(pin int, mode int) error {
	if err := b.write([]byte{PinMode, byte(pin), byte(mode)}); err != nil {
		return err
	}
	b.pins[byte(pin)].Mode = mode
	return nil
}

// DigitalWrite writes value to pin.
func (b *Client) DigitalWrite(pin int, value int) error {
	port := byte(math.Floor(float64(pin) / 8))
	portValue := byte(0)

	b.pins[pin].Value = value

	for i := byte(0); i < 8; i++ {
		if b.pins[8*port+i].Value != 0 {
			portValue = portValue | (1 << i)
		}
	}
	return b.write([]byte{DigitalMessage | port, portValue & 0x7F, (portValue >> 7) & 0x7F})
}

// ServoConfig sets the min and max pulse width for servo PWM range
func (b *Client) ServoConfig(pin int, max int, min int) error {
	ret := []byte{
		ServoConfig,
		byte(pin),
		byte(max & 0x7F),
		byte((max >> 7) & 0x7F),
		byte(min & 0x7F),
		byte((min >> 7) & 0x7F),
	}
	return b.writeSysex(ret)
}

// AnalogWrite writes value to pin.
func (b *Client) AnalogWrite(pin int, value int) error {
	if err := b.write([]byte{
		AnalogMessage | byte(pin),
		byte(value & 0x7F),
		byte((value >> 7) & 0x7F),
	}); err != nil {
		return err
	}
	b.pins[pin].Value = value
	return nil
}

// FirmwareQuery sends the FirmwareQuery sysex code.
func (b *Client) FirmwareQuery() (chan bool, error) {
	if err := b.writeSysex([]byte{FirmwareQuery}); err != nil {
		return nil, err
	}
	b.boolChan["FirmwareQuery"] = make(chan bool)
	return b.boolChan["FirmwareQuery"], nil
}

// PinStateQuery sends a PinStateQuery for pin.
func (b *Client) PinStateQuery(pin int) (chan Pin, error) {
	if err := b.writeSysex([]byte{PinStateQuery, byte(pin)}); err != nil {
		return nil, err
	}
	b.pinStateChan[pin] = make(chan Pin)
	return b.pinStateChan[pin], nil
}

// ProtocolVersionQuery sends the ProtocolVersion sysex code.
func (b *Client) ProtocolVersionQuery() (chan bool, error) {
	if err := b.write([]byte{ProtocolVersion}); err != nil {
		return nil, err
	}
	b.boolChan["ProtocolVersion"] = make(chan bool)
	return b.boolChan["ProtocolVersion"], nil
}

// CapabilityQuery sends the CapabilityQuery sysex code.
func (b *Client) CapabilityQuery() (chan bool, error) {
	if err := b.writeSysex([]byte{CapabilityQuery}); err != nil {
		return nil, err
	}
	b.boolChan["CapabilityQuery"] = make(chan bool)
	return b.boolChan["CapabilityQuery"], nil
}

// AnalogMappingQuery sends the AnalogMappingQuery sysex code.
func (b *Client) AnalogMappingQuery() (chan bool, error) {
	if err := b.writeSysex([]byte{AnalogMappingQuery}); err != nil {
		return nil, err
	}
	b.boolChan["AnalogMappingQuery"] = make(chan bool)
	return b.boolChan["AnalogMappingQuery"], nil
}

// ReportDigital enables or disables digital reporting for pin, a non zero
// state enables reporting
func (b *Client) ReportDigital(pin int, state int) (chan Pin, error) {
	return b.togglePinReporting(pin, state, ReportDigital)
}

// ReportAnalog enables or disables analog reporting for pin, a non zero
// state enables reporting
func (b *Client) ReportAnalog(pin int, state int) (chan Pin, error) {
	return b.togglePinReporting(pin, state, ReportAnalog)
}

// I2cRead reads numBytes from address once.
func (b *Client) I2cRead(address int, numBytes int) (chan I2cReply, error) {
	if err := b.writeSysex([]byte{I2CRequest, byte(address), (I2CModeRead << 3),
		byte(numBytes) & 0x7F, (byte(numBytes) >> 7) & 0x7F}); err != nil {
		return nil, err
	}
	b.i2cChan = make(chan I2cReply)
	return b.i2cChan, nil
}

// I2cWrite writes data to address.
func (b *Client) I2cWrite(address int, data []byte) error {
	ret := []byte{I2CRequest, byte(address), (I2CModeWrite << 3)}
	for _, val := range data {
		ret = append(ret, byte(val&0x7F))
		ret = append(ret, byte((val>>7)&0x7F))
	}
	return b.writeSysex(ret)
}

// I2cConfig configures the delay in which a register can be read from after it
// has been written to.
func (b *Client) I2cConfig(delay int) error {
	return b.writeSysex([]byte{I2CConfig, byte(delay & 0xFF), byte((delay >> 8) & 0xFF)})
}

func (b *Client) togglePinReporting(pin int, state int, mode byte) (chan Pin, error) {
	if state != 0 {
		state = 1
	} else {
		state = 0
	}

	if err := b.write([]byte{byte(mode) | byte(pin), byte(state)}); err != nil {
		return nil, err
	}

	b.pinReportChan[pin] = make(chan Pin)

	return b.pinReportChan[pin], nil

}

func (b *Client) writeSysex(data []byte) (err error) {
	return b.write(append([]byte{StartSysex}, append(data, EndSysex)...))
}

func (b *Client) write(data []byte) (err error) {
	_, err = b.connection.Write(data[:])
	return
}

func (b *Client) read(length int) (buf []byte, err error) {
	i := 0
	for length > 0 {
		tmp := make([]byte, length)
		if i, err = b.connection.Read(tmp); err != nil {
			if err.Error() != "EOF" {
				return
			}
			<-time.After(5 * time.Millisecond)
		}
		if i > 0 {
			buf = append(buf, tmp...)
			length = length - i
		}
	}
	return
}

func (b *Client) process() (err error) {
	buf, err := b.read(3)
	if err != nil {
		return err
	}
	messageType := buf[0]
	switch {
	case ProtocolVersion == messageType:
		b.protocolVersion = fmt.Sprintf("%v.%v", buf[1], buf[2])

		select {
		case b.boolChan["ProtocolVersion"] <- true:
		default:
		}
	case AnalogMessageRangeStart <= messageType &&
		AnalogMessageRangeEnd >= messageType:

		value := uint(buf[1]) | uint(buf[2])<<7
		pin := int((messageType & 0x0F))

		if len(b.analogPins) > pin {
			if len(b.pins) > b.analogPins[pin] {
				b.pins[b.analogPins[pin]].Value = int(value)
				select {
				case b.pinReportChan[b.analogPins[pin]] <- b.pins[b.analogPins[pin]]:
				default:
				}
			}
		}
	case DigitalMessageRangeStart <= messageType &&
		DigitalMessageRangeEnd >= messageType:

		port := messageType & 0x0F
		portValue := buf[1] | (buf[2] << 7)

		for i := 0; i < 8; i++ {
			pinNumber := int((8*byte(port) + byte(i)))
			if len(b.pins) > pinNumber {
				if b.pins[pinNumber].Mode == Input {
					b.pins[pinNumber].Value = int((portValue >> (byte(i) & 0x07)) & 0x01)
					select {
					case b.pinReportChan[pinNumber] <- b.pins[pinNumber]:
					default:
					}
				}
			}
		}
	case StartSysex == messageType:
		currentBuffer := buf
		for {
			buf, err = b.read(1)
			if err != nil {
				return err
			}
			currentBuffer = append(currentBuffer, buf[0])
			if buf[0] == EndSysex {
				break
			}
		}
		command := currentBuffer[1]
		switch command {
		case CapabilityResponse:
			b.pins = []Pin{}
			supportedModes := 0
			n := 0

			for _, val := range currentBuffer[2:(len(currentBuffer) - 5)] {
				if val == 127 {
					modes := []int{}
					for _, mode := range []int{Input, Output, Analog, Pwm, Servo} {
						if (supportedModes & (1 << byte(mode))) != 0 {
							modes = append(modes, mode)
						}
					}

					b.pins = append(b.pins, Pin{SupportedModes: modes, Mode: Output})
					supportedModes = 0
					n = 0
					continue
				}

				if n == 0 {
					supportedModes = supportedModes | (1 << val)
				}
				n ^= 1
			}
			select {
			case b.boolChan["CapabilityQuery"] <- true:
			default:
			}
		case AnalogMappingResponse:
			pinIndex := 0
			b.analogPins = []int{}

			for _, val := range currentBuffer[2 : len(b.pins)-1] {

				b.pins[pinIndex].AnalogChannel = int(val)

				if val != 127 {
					b.analogPins = append(b.analogPins, pinIndex)
				}
				pinIndex++
			}
			select {
			case b.boolChan["AnalogMappingQuery"] <- true:
			default:
			}
		case PinStateResponse:
			pin := int(currentBuffer[2])
			b.pins[pin].Mode = int(currentBuffer[3])
			b.pins[pin].State = int(currentBuffer[4])

			if len(currentBuffer) > 6 {
				b.pins[pin].State = int(uint(b.pins[pin].State) | uint(currentBuffer[5])<<7)
			}
			if len(currentBuffer) > 7 {
				b.pins[pin].State = int(uint(b.pins[pin].State) | uint(currentBuffer[6])<<14)
			}

			select {
			case b.pinStateChan[pin] <- b.pins[pin]:
			default:
			}
		case I2CReply:
			reply := I2cReply{
				Address:  int(byte(currentBuffer[2]) | byte(currentBuffer[3])<<7),
				Register: int(byte(currentBuffer[4]) | byte(currentBuffer[5])<<7),
				Data:     []byte{byte(currentBuffer[6]) | byte(currentBuffer[7])<<7},
			}
			for i := 8; i < len(currentBuffer); i = i + 2 {
				if currentBuffer[i] == byte(0xF7) {
					break
				}
				if i+2 > len(currentBuffer) {
					break
				}
				reply.Data = append(reply.Data,
					byte(currentBuffer[i])|byte(currentBuffer[i+1])<<7,
				)
			}
			select {
			case b.i2cChan <- reply:
			default:
			}
		case FirmwareQuery:
			name := []byte{}
			for _, val := range currentBuffer[4:(len(currentBuffer) - 1)] {
				if val != 0 {
					name = append(name, val)
				}
			}
			b.firmwareName = string(name[:])
			select {
			case b.boolChan["FirmwareQuery"] <- true:
			default:
			}
		case StringData:
			str := currentBuffer[2:len(currentBuffer)]
			select {
			case b.stringDataChan <- string(str[:len(str)-1]):
			default:
			}
		}
	}
	return
}
