/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
	result, err := privateData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading private data list")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
