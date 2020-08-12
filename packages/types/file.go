/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types
	f.Set("Name", v)
	if v, ok = m["MimeType"].(string); !ok {
		return
	}
	f.Set("MimeType", v)
	if v, ok = m["Body"].([]byte); !ok {
		return
	}
	f.Set("Body", v)

	return
}
