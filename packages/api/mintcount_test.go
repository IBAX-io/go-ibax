/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	"github.com/IBAX-io/go-ibax/packages/model"
	"testing"
)

func TestMineCount(t *testing.T) {
	var ret model.Response
	err := sendGet(`mintcount/163`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}

}
