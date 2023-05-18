/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

type DebugApi struct {
	de *debugApi
}

func (p *DebugApi) GetApis() []any {
	var apis []any
	if p == nil {
		return nil
	}
	if p.de != nil {
		apis = append(apis, p.de)
	}

	return apis
}

func NewDebugApi() *DebugApi {
	return &DebugApi{
		de: newDebugApi(),
	}
}
