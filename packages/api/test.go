/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.

	"github.com/gorilla/mux"
)

type getTestResult struct {
	Value string `json:"value"`
}

func getTestHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	jsonResponse(w, &getTestResult{
		Value: smart.GetTestValue(params["name"]),
	})
}
