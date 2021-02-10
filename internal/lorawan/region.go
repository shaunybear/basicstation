package lorawan

import "fmt"

// Region enumerates LoRaWAN Region
type Region uint8

const (
	// US902 LoRaWAN Region
	US902 Region = iota + 1
	// EU863 LoRaWAN Region
	EU863
	// IN865 LoRaWAN Region
	IN865
	// AS923 LoRaWAN Region
	AS923
	// AU915 LoRaWAN Region
	AU915

	// NofRegions counts the number of regions. Make sure this is
	// next value after the final region
	NofRegions uint = iota
)

// SpreadingFactor enumerates Lora spreading factor
type SpreadingFactor uint8

const (
	// SF7 Spreading Factor
	SF7 SpreadingFactor = iota + 7
	// SF8 Spreading Factor
	SF8
	// SF9 Spreading Factor
	SF9
	// SF10 Spreading Factor
	SF10
	// SF11 Spreading Factor
	SF11
	// SF12 Spreading Factor
	SF12
)

// Bandwidth enumerates Lora bandwidths
type Bandwidth uint8

const (
	// BW125  125 KHz
	BW125 Bandwidth = iota + 1
	// BW250  250 KHz
	BW250
	// BW500  500 KHz
	BW500
)

// Stringer returns the BasicStation region string
func (r Region) Stringer() string {
	return [...]string{"Unknown", "US902", "EU863", "IN865", "AS923", "AU915"}[r]
}

// Datarate is LoRaWAN Datarate
type Datarate struct {
	SF SpreadingFactor
	BW Bandwidth
}

// RegionParams contains LoRaWAN regional parameters
type RegionParams struct {
	Region
	DRs        []Datarate
	MaxTxPower uint8
	FreqRange  []uint
}

// GetRegionalParams returns an instance of the regional parameters
// for the specified region
func GetRegionalParams(r Region) (params RegionParams, err error) {
	switch r {
	case US902:
		params = newUS902Region()
	default:
		err = fmt.Errorf("%v region not implemented", r)
	}
	return
}
