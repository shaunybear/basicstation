package basicstation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

// DataDown message
type DataDown struct {
	r  io.Reader
	mt int
}

// Environment ...
type Environment struct {
	Server Server
	Log    zerolog.Logger
}

// RxContext common uplink/downlink radio fields
type RxContext struct {
	RCTX    int64 `json:"rctx"`
	XTime   int64 `json:"xtime"`
	GPSTime int64 `json:"gpstime"`
}

// Version message reports version infomration to the LNS
type Version struct {
	Station  string `json:"station"`
	Firmware string `json:"firmware"`
	Package  string `json:"package"`
	Model    string `json:"model"`
	Protocol uint   `json:"protocol"`
	Features string `json:"features"`
	MsgType  string `json:"msgtype"`
}

// UpInfo message  present in all radio frames
type UpInfo struct {
	RSSI float64   `json:"rssi"`
	SNR  float64   `json:"snr"`
	RCtx RxContext `mapstructure:",squash"`
}

// JoinRequest message is a parsed join request
type JoinRequest struct {
	MsgType  string `json:"msgtype"`
	MHdr     uint8
	JoinEUI  string `json:"JoinEui"`
	DevEUI   string `json:"DevEui"`
	DevNonce uint16
	MIC      int32
	DR       int
	Freq     int
	UpInfo   UpInfo
}

// Uplink encodes an uplink frame
type Uplink struct {
	MHdr       uint8
	DevAddr    uint32
	FCtrl      uint8
	FCnt       uint16
	FOpts      string
	FPort      int8
	FRMPayload string
	MIC        int32
	DR         int
	Freq       int
	UpInfo     UpInfo
	MsgType    string `json:"msgtype"`
}

// Downlink encodes a downlink frame
type Downlink struct {
	DevEui   string
	PDU      string `json:"pdu"`
	RxDelay  int
	RX1DR    int
	RX1Freq  int
	RX2DR    int
	RX2Freq  int
	Priority int
	Xtime    int64  `json:"xtime"`
	Rxtx     int64  `json:"rctx"`
	MsgType  string `json:"msgtype"`
}

// Implement io.Reader interface
func (uplink Uplink) Read(b []byte) (int, error) {
	b, err := json.Marshal(&uplink)
	return len(b), err
}

// DnTxed is the basic station transmit confirmation message
type DnTxed struct {
	DIID   int64     `json:"diid"`
	DevEUI uint64    `json:"DevEui"`
	TXTime float64   `json:"txtime"`
	RCtx   RxContext `mapstructure:",squash"`
}

// RadioChannel defines an SX1301 channel configuration
type RadioChannel struct {
	Enable bool `json:"enable"`
	Radio  uint `json:"radio"`
	IF     int  `json:"if"`
}

// LoraStdChannel is a Radio channel with additional parameters
type LoraStdChannel struct {
	RadioChannel
	Bandwidth       int `json:"bandwidth"`
	SpreadingFactor int `json:"spread_factor"`
}

// Radio is an SX1301 radio configuration
type Radio struct {
	Enable bool   `json:"enable"`
	Freq   uint32 `json:"freq"`
}

// SX1301 defines how the channel plan maps to the individual SX1301 chips
type SX1301 struct {
	Radio0      Radio          `json:"radio_0"`
	Radio1      Radio          `json:"radio_1"`
	Channel0    RadioChannel   `json:"chan_multiSF_0"`
	Channel1    RadioChannel   `json:"chan_multiSF_1"`
	Channel2    RadioChannel   `json:"chan_multiSF_2"`
	Channel3    RadioChannel   `json:"chan_multiSF_3"`
	Channel4    RadioChannel   `json:"chan_multiSF_4"`
	Channel5    RadioChannel   `json:"chan_multiSF_5"`
	Channel6    RadioChannel   `json:"chan_multiSF_6"`
	Channel7    RadioChannel   `json:"chan_multiSF_7"`
	ChannelLora LoraStdChannel `json:"chan_Lora_std"`
	ChannelFSK  RadioChannel   `json:"chan_FSK"`
}

// RouterConf message specifies a channelplan for the station and defines
// some basic operation modes
type RouterConf struct {
	MessageType string   `json:"msgtype"`
	DRs         [][]int  `json:",omitempty"`
	NetID       [][]uint `json:",omitempty"`
	JoinEUI     [][]uint `json:"JoinEui,omitempty"`
	Region      string   `json:"region"`
	HWSPEC      string   `json:"hwspec"`
	FreqRange   []uint   `json:"freq_range,omitempty"`
	SX1301s     []SX1301 `json:"SX1301_conf,omitempty"`
	NOCCA       bool     `json:"nocca"`
	NODC        bool     `json:"nodc"`
	NODWELL     bool     `json:"nodwell"`
}

// UnsupportedMsgType error
type UnsupportedMsgType struct {
	mtype string
}

const (
	// RouterConfMsgName is the router config message type field value
	RouterConfMsgName = "router_config"
)

// Error satisifies error interface
func (u UnsupportedMsgType) Error() string {
	return fmt.Sprintf("unsupported message type: %s", u.mtype)
}

// decode decodes a basic station message
func decode(r io.Reader) (interface{}, error) {
	input := map[string]interface{}{}
	var output interface{}

	dec := json.NewDecoder(r)
	err := dec.Decode(&input)
	if err != nil {
		return nil, err
	}

	mt, ok := input["msgtype"]
	if !ok {
		return nil, fmt.Errorf("no msgtype in %v", input)
	}

	switch mt.(type) {
	case string:
		switch mt.(string) {
		case "jreq":
			output = JoinRequest{}
		case "updf":
			output = Uplink{}
		case "dntxed":
			output = DnTxed{}
		case "version":
			output = Version{}
		case "propdf":
			// ignore
		default:
			err := UnsupportedMsgType{mtype: string(mt.(string))}
			return nil, err
		}
	default:
		return nil, fmt.Errorf("msgtype is not a string")
	}

	if err := mapstructure.Decode(&input, &output); err != nil {
		return nil, err
	}

	return output, nil
}

// Encode json encodes the input and wraps it in a io.Reader
func Encode(msg interface{}) (io.Reader, error) {
	b, err := json.Marshal(&msg)
	if err != nil {
		return nil, err
	}

	return io.LimitReader(bytes.NewReader(b), int64(len(b))), nil
}
