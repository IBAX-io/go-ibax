/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

	if vm == nil {
		return nil
	}
	client := getClient(r)
	contract := smart.VMGetContract(vm, name, uint32(client.EcosystemID))
	if contract == nil {
		return nil
	}
	return contract
}

func getContractInfo(contract *smart.Contract) *script.ContractInfo {
	return contract.Block.Info.(*script.ContractInfo)
}
