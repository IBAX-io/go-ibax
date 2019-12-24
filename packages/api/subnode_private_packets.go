/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading private data list")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
