/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
func TestMineCount(t *testing.T) {
	var ret model.Response
	err := sendGet(`mintcount/163`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}

}
