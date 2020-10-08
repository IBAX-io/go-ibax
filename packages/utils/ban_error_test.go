/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	"github.com/stretchr/testify/assert"
)

func TestBanError(t *testing.T) {
	cases := map[error]bool{
		errors.New("case 1"):                                  false,
		WithBan(errors.New("case 2")):                         true,
		errors.Wrap(errors.New("case 3"), "message"):          false,
		errors.Wrap(WithBan(errors.New("case 4")), "message"): true,
	}

	for err, ok := range cases {
		assert.Equal(t, ok, IsBanError(err), err.Error())
	}
}
