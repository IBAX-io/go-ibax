 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

func getVersionHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, strings.TrimSpace(strings.Join([]string{
		consts.VERSION, consts.BuildInfo}, " ",
	)))
}
