/*---------------------------------------------------------------------------------------------

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/smart"

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
