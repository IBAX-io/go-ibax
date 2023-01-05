/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/node"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const corsMaxAge = 600

type Router struct {
	main        *mux.Router
	apiVersions map[string]*mux.Router
}

func (r Router) GetAPI() *mux.Router {
	return r.main
}

func (r Router) GetAPIVersion(preffix string) *mux.Router {
	return r.apiVersions[preffix]
}

func (r Router) NewVersion(preffix string) *mux.Router {
	api := r.main.PathPrefix(preffix).Subrouter()
	r.apiVersions[preffix] = api
	return api
}

// Route sets routing pathes
func (m Mode) SetCommonRoutes(r Router) {
	r.main.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		jsonResponse(writer, consts.Version()+" "+node.NodePauseType().String())
	}).Methods("get")
	api := r.NewVersion("/api/v2")

	api.Use(nodeStateMiddleware, tokenMiddleware, m.clientMiddleware)

	SetOtherCommonRoutes(api, m)
	api.HandleFunc("/data/{id}/data/{hash}", getBinaryHandler).Methods("GET")
	api.HandleFunc("/data/{table}/{id}/{column}/{hash}", getDataHandler).Methods("GET")
	api.HandleFunc("/avatar/{ecosystem}/{account}", getAvatarHandler).Methods("GET")
	api.HandleFunc("/auth/status", getAuthStatus).Methods("GET")

	api.HandleFunc("/contract/{name}", authRequire(getContractInfoHandler)).Methods("GET")
	api.HandleFunc("/contracts", authRequire(getContractsHandler)).Methods("GET")
	api.HandleFunc("/getuid", getUIDHandler).Methods("GET")
	api.HandleFunc("/keyinfo/{wallet}", m.getKeyInfoHandler).Methods("GET")
	api.HandleFunc("/list/{name}", authRequire(getListHandler)).Methods("GET")
	api.HandleFunc("/network", getNetworkHandler).Methods("GET")
	api.HandleFunc("/sections", authRequire(getSectionsHandler)).Methods("GET")
	api.HandleFunc("/row/{name}/{id}", authRequire(getRowHandler)).Methods("GET")
	api.HandleFunc("/row/{name}/{column}/{id}", authRequire(getRowHandler)).Methods("GET")
	api.HandleFunc("/interface/page/{name}", authRequire(getPageRowHandler)).Methods("GET")
	api.HandleFunc("/interface/menu/{name}", authRequire(getMenuRowHandler)).Methods("GET")
	api.HandleFunc("/interface/snippet/{name}", authRequire(getSnippetRowHandler)).Methods("GET")
	api.HandleFunc("/table/{name}", authRequire(getTableHandler)).Methods("GET")
	api.HandleFunc("/tables", authRequire(getTablesHandler)).Methods("GET")
	api.HandleFunc("/version", getVersionHandler).Methods("GET")
	api.HandleFunc("/config/{option}", getConfigOptionHandler).Methods("GET")

	api.HandleFunc("/page/validators_count/{name}", getPageValidatorsCountHandler).Methods("GET")
	api.HandleFunc("/content/source/{name}", authRequire(getSourceHandler)).Methods("POST")
	api.HandleFunc("/content/page/{name}", authRequire(getPageHandler)).Methods("POST")
	api.HandleFunc("/content/hash/{name}", getPageHashHandler).Methods("POST")
	api.HandleFunc("/content/menu/{name}", authRequire(getMenuHandler)).Methods("POST")
	api.HandleFunc("/content", jsonContentHandler).Methods("POST")
	api.HandleFunc("/login", m.loginHandler).Methods("POST")
	api.HandleFunc("/sendTx", authRequire(m.sendTxHandler)).Methods("POST")
	api.HandleFunc("/node/{name}", nodeContractHandler).Methods("POST")
	api.HandleFunc("/txstatus", authRequire(getTxStatusHandler)).Methods("POST")
	api.HandleFunc("/metrics/blocks", blocksCountHandler).Methods("GET")
	api.HandleFunc("/metrics/transactions", txCountHandler).Methods("GET")
	api.HandleFunc("/metrics/ecosystems", m.ecosysCountHandler).Methods("GET")
	api.HandleFunc("/metrics/keys", keysCountHandler).Methods("GET")
	api.HandleFunc("/metrics/mem", memStatHandler).Methods("GET")
	api.HandleFunc("/metrics/ban", banStatHandler).Methods("GET")

}

func (m Mode) SetBlockchainRoutes(r Router) {
	api := r.GetAPIVersion("/api/v2")
	setOtherBlockChainRoutes(api, m)
	api.HandleFunc("/metrics/honornodes", honorNodesCountHandler).Methods("GET")
	api.HandleFunc("/txinfo/{hash}", getTxInfoHandler).Methods("GET")
	api.HandleFunc("/txinfomultiple", getTxInfoMultiHandler).Methods("GET")
	api.HandleFunc("/appparam/{appID}/{name}", authRequire(m.GetAppParamHandler)).Methods("GET")
	api.HandleFunc("/appparams/{appID}", authRequire(m.getAppParamsHandler)).Methods("GET")
	api.HandleFunc("/appcontent/{appID}", authRequire(m.getAppContentHandler)).Methods("GET")
	api.HandleFunc("/history/{name}/{id}", authRequire(getHistoryHandler)).Methods("GET")
	api.HandleFunc("/balance/{wallet}", m.getBalanceHandler).Methods("GET")
	api.HandleFunc("/block/{id}", getBlockInfoHandler).Methods("GET")
	api.HandleFunc("/maxblockid", getMaxBlockHandler).Methods("GET")
	api.HandleFunc("/blocks", getBlocksTxInfoHandler).Methods("GET")
	api.HandleFunc("/detailed_blocks", getBlocksDetailedInfoHandler).Methods("GET")
	api.HandleFunc("/ecosystemparams", authRequire(m.getEcosystemParamsHandler)).Methods("GET")
	api.HandleFunc("/systemparams", authRequire(getPlatformParamsHandler)).Methods("GET")
	api.HandleFunc("/ecosystemparam/{name}", authRequire(m.getEcosystemParamHandler)).Methods("GET")
	api.HandleFunc("/ecosystemname", getEcosystemNameHandler).Methods("GET")
}

func SetOtherCommonRoutes(api *mux.Router, m Mode) {
	api.HandleFunc("/member/{ecosystem}/{account}", getMemberHandler).Methods("GET")
	api.HandleFunc("/listWhere/{name}", authRequire(getListWhereHandler)).Methods("POST")
	api.HandleFunc("/nodelistWhere/{name}", authRequire(getnodeListWhereHandler)).Methods("POST")
	api.HandleFunc("/sumWhere/{name}", authRequire(getsumWhereHandler)).Methods("POST")
	api.HandleFunc("/metrics/blockper/{node}", blocksCountByNodeHandler).Methods("GET")
}

func setOtherBlockChainRoutes(api *mux.Router, m Mode) {
	api.HandleFunc("/tx_record/{hashes}", getTxRecord).Methods("GET")
}

func (m Mode) SetSubNodeRoutes(r Router) {}

func NewRouter(m Mode) Router {
	r := mux.NewRouter()
	r.StrictSlash(true)
	r.Use(loggerMiddleware, recoverMiddleware, statsdMiddleware)

	api := Router{
		main:        r,
		apiVersions: make(map[string]*mux.Router),
	}
	m.SetCommonRoutes(api)
	return api
}

func WithCors(h http.Handler) http.Handler {
	return handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "HEAD", "POST"}),
		handlers.AllowedHeaders([]string{"Authorization", "Content-Type", "X-Requested-With"}),
		handlers.MaxAge(corsMaxAge),
	)(h)
}
