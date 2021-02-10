package lorawan

func newUS902Region() RegionParams {
	r := RegionParams{Region: US902}
	r.MaxTxPower = 30
	r.FreqRange = []uint{902000000, 928000000}
	r.DRs = []Datarate{
		// DR0
		{
			SF: SF10,
			BW: BW125,
		},

		// DR1
		{
			SF: SF9,
			BW: BW125,
		},
		// DR2
		{
			SF: SF8,
			BW: BW125,
		},
		// DR3
		{
			SF: SF7,
			BW: BW125,
		},
		// DR4
		{
			SF: SF8,
			BW: BW500,
		},
		// 5 -7 not used
		{},
		{},
		{},
		// DR8
		{
			SF: SF12,
			BW: BW500,
		},
		// DR9
		{
			SF: SF11,
			BW: BW500,
		},
		// DR10
		{
			SF: SF10,
			BW: BW500,
		},
		// DR11
		{
			SF: SF9,
			BW: BW500,
		},
		// DR12
		{
			SF: SF8,
			BW: BW500,
		},
		//DR13
		{
			SF: SF8,
			BW: BW500,
		},
		//DR 14,15 - Not used
		{}, {},
	}
	return r
}
