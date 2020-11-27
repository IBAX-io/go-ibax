module ibax.io/langcode

go 1.15

require (
	github.com/360EntSecGroup-Skylar/excelize v1.4.1
	github.com/pkg/errors v0.9.1
	github.com/shopspring/decimal v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	gopkg.in/vmihailenco/msgpack.v2 v2.9.1
	ibax.io/common v0.0.0
	ibax.io/conf v0.0.0
	ibax.io/crypto v0.0.0
	ibax.io/deprecated v0.0.0
	ibax.io/miner v0.0.0
	ibax.io/obsmanager v0.0.0
	ibax.io/scheduler v0.0.0
	ibax.io/store v0.0.0
	ibax.io/vm v0.0.0
)

replace (
	ibax.io/common => ../common
	ibax.io/conf => ../conf
	ibax.io/crypto => ../crypto
	ibax.io/deprecated => ../deprecated
	ibax.io/miner => ../miner
	ibax.io/obsmanager => ../obsmanager
	ibax.io/scheduler => ../scheduler
	ibax.io/store => ../store
	ibax.io/vm => ../vm
)
