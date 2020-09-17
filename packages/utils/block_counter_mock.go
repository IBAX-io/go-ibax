/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	ret := _m.Called(state)

	var r0 int
	if rf, ok := ret.Get(0).(func(blockGenerationState) int); ok {
		r0 = rf(state)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(blockGenerationState) error); ok {
		r1 = rf(state)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
