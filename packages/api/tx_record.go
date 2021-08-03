/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
)

func getTxRecord(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	hashes := params["hashes"]

	var (
		hashList   []string
		resultList []interface{}
	)
	if len(hashes) > 0 {
		hashList = strings.Split(hashes, ",")
	}
	for _, hashStr := range hashList {

		if result, err := model.GetTxRecord(nil, hashStr); err == nil {
			resultList = append(resultList, result)
		}
	}
	jsonResponse(w, &resultList)
	return
}
