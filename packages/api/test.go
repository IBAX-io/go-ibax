/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

}

func getTestHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	jsonResponse(w, &getTestResult{
		Value: smart.GetTestValue(params["name"]),
	})
}
