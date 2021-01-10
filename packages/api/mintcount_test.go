/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
)

func TestMineCount(t *testing.T) {
	var ret model.Response
	err := sendGet(`mintcount/163`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}

}
