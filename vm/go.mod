module ibax.io/vm

go 1.15

require (
	github.com/shopspring/decimal v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	ibax.io/common v0.0.0
	ibax.io/crypto v0.0.0
	ibax.io/store v0.0.0
)

replace (
	ibax.io/common => ../common
	ibax.io/conf => ../conf
	ibax.io/crypto => ../crypto
	ibax.io/store => ../store

)
