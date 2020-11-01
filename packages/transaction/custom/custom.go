 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package custom

import (
	"github.com/IBAX-io/go-ibax/packages/utils/tx"
)

// TransactionInterface is parsing transactions
type TransactionInterface interface {
	Init() error
	Validate() error
	Action() error
	Rollback() error
	Header() *tx.Header
}
