/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package httpserver

import "net/http"


func NewMaxBodyReader(h http.Handler, n int64) http.Handler {
	return &MaxBodyReader{h, n}
}
