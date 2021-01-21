/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
)

func getContract(r *http.Request, name string) *smart.Contract {
	return contract
}

func getContractInfo(contract *smart.Contract) *script.ContractInfo {
	return contract.Block.Info.(*script.ContractInfo)
}
