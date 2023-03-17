/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

type IbaxApi struct {
	auth    *authApi
	bk      *blockChainApi
	common  *commonApi
	tx      *transactionApi
	account *accountsApi
}

func (p *IbaxApi) GetApis() []any {
	var apis []any
	if p == nil {
		return nil
	}
	if p.auth != nil {
		apis = append(apis, p.auth)
	}
	if p.bk != nil {
		apis = append(apis, p.bk)
	}
	if p.common != nil {
		apis = append(apis, p.common)
	}
	if p.tx != nil {
		apis = append(apis, p.tx)
	}
	if p.account != nil {
		apis = append(apis, p.account)
	}
	return apis
}

func NewIbaxApi(m Mode) *IbaxApi {
	return &IbaxApi{
		auth:    NewAuthApi(m),
		bk:      NewBlockChainApi(),
		common:  NewCommonApi(m),
		tx:      NewTransactionApi(),
		account: NewAccountsApi(m),
	}
}
