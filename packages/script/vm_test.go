/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
		v   interface{}
		mem int64
	}{
		{true, 1},
		{int8(1), 1}, {int16(1), 2}, {int32(1), 4},
		{int64(1), 8}, {int(1), 8},
		{float32(1), 4}, {float64(1), 8},
		{"test", 4},
		{[]byte("test"), 16},
		{[]string{"test", "test"}, 20},
		{map[string]string{"test": "test"}, 12},
	}

	for _, v := range cases {
		assert.Equal(t, v.mem, calcMem(v.v))
	}
}
