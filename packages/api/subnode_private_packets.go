/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

)

func privateDataListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	privateData := model.PrivatePackets{}

	result, err := privateData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading private data list")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
