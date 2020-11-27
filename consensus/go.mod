module ibax.io/consensus

go 1.15

require (
	github.com/stretchr/testify v1.6.1
	ibax.io/store v0.0.0
)

replace (
	ibax.io/common => ../common
	ibax.io/conf => ../conf
	ibax.io/crypto => ../crypto
	ibax.io/store => ../store
)
