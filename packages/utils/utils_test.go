/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

		{"HMac", "h_mac"},
		{"JSONEncode", "json_encode"},
		{"Hash", "hash"},
		{"PubToID", "pub_to_id"},
	}
	for _, tt := range cases {
		assert.Equal(t, tt.expected, ToSnakeCase(tt.arg))
	}
}

func TestNtp(t *testing.T) {
	for i := 0; i < 1000; i++ {
		st := time.Now()
		b, err := CheckClockDrift()
		et := time.Now()
		dr := et.Sub(st)
		fmt.Println("dr:" + dr.String())
		assert.Error(t, err, nil)
		if b {
			fmt.Println("time ok")
		} else {
			fmt.Println("time not ok")
		}
		time.Sleep(2 * time.Second)
	}
}
