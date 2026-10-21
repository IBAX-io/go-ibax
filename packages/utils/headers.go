/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package utils

import "net/http"

// credentialHeaders carry what signs a client in; their values never go into a log
var credentialHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie"}

// LogHeaders returns a copy of the request headers to log, with the values of the credential
// headers hidden
func LogHeaders(h http.Header) http.Header {
	out := h.Clone()
	for _, name := range credentialHeaders {
		if len(out.Values(name)) > 0 {
			out.Set(name, "[hidden]")
		}
	}
	return out
}
