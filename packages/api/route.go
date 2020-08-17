/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

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
	api := r.NewVersion("/api/v2")

	api.Use(nodeStateMiddleware, tokenMiddleware, m.clientMiddleware)

	SetOtherCommonRoutes(api, m)
	api.HandleFunc("/data/{prefix}_binaries/{id}/data/{hash}", getBinaryHandler).Methods("GET")
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
	api.HandleFunc("/interface/block/{name}", authRequire(getBlockInterfaceRowHandler)).Methods("GET")
	api.HandleFunc("/table/{name}", authRequire(getTableHandler)).Methods("GET")
	api.HandleFunc("/tables", authRequire(getTablesHandler)).Methods("GET")
	api.HandleFunc("/test/{name}", getTestHandler).Methods("GET", "POST")
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
	api.HandleFunc("/sendSignTx", m.sendSignTxHandler).Methods("POST")
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
	api.HandleFunc("/txinfo/{hash}", authRequire(getTxInfoHandler)).Methods("GET")
	api.HandleFunc("/txinfomultiple", authRequire(getTxInfoMultiHandler)).Methods("GET")
	api.HandleFunc("/appparam/{appID}/{name}", authRequire(m.GetAppParamHandler)).Methods("GET")
	api.HandleFunc("/appparams/{appID}", authRequire(m.getAppParamsHandler)).Methods("GET")
	api.HandleFunc("/appcontent/{appID}", authRequire(m.getAppContentHandler)).Methods("GET")
	api.HandleFunc("/history/{name}/{id}", authRequire(getHistoryHandler)).Methods("GET")
	api.HandleFunc("/balance/{wallet}", authRequire(m.getBalanceHandler)).Methods("GET")
	api.HandleFunc("/assignbalance/{wallet}", authRequire(m.getMyAssignBalanceHandler)).Methods("GET")
	api.HandleFunc("/block/{id}", getBlockInfoHandler).Methods("GET")
	api.HandleFunc("/maxblockid", getMaxBlockHandler).Methods("GET")
	api.HandleFunc("/sumWhere/{name}", authRequire(getsumWhereHandler)).Methods("POST")
	api.HandleFunc("/metrics/blockper/{node}", blocksCountByNodeHandler).Methods("GET")
	// Open database data APIS
	api.HandleFunc("/open/databaseInfo", getOpenDatabaseInfoHandler).Methods("POST")
	api.HandleFunc("/open/tablesInfo", getOpenTablesInfoHandler).Methods("POST")
	api.HandleFunc("/open/columnsInfo", getOpenColumnsInfoHandler).Methods("POST")
	api.HandleFunc("/open/rowsInfo", getOpenRowsInfoHandler).Methods("POST")

}

func (m Mode) SetGafsRoutes(r Router) {

	gafs := r.GetAPIVersion("/api/v2").PathPrefix("/gafs").Subrouter()

	gafs.HandleFunc("/file_pre", authRequire(filePre)).Methods("POST")
	gafs.HandleFunc("/add", authRequire(add)).Methods("POST")
	//gafs.HandleFunc("/add_dir", authRequire(addDir)).Methods("POST")
	gafs.HandleFunc("/cat/{hash}", authRequire(cat)).Methods("GET")

	gafs.HandleFunc("/files/mkdir", authRequire(filesMkdir)).Methods("POST")
	gafs.HandleFunc("/files/stat", authRequire(filesStat)).Methods("POST")
	gafs.HandleFunc("/files/rm", authRequire(filesRm)).Methods("POST")
	gafs.HandleFunc("/files/mv", authRequire(filesMv)).Methods("POST")
	gafs.HandleFunc("/files/cp/{hash}", authRequire(filesCp)).Methods("POST")

	gafs.HandleFunc("/files/ls", authRequire(filesLs)).Methods("POST")
	//gafs.HandleFunc("/file/ls/{hash}", authRequire(fileLs)).Methods("GET")
	//gafs.HandleFunc("/ls/{hash}", authRequire(ls)).Methods("GET")
}

func setOtherBlockChainRoutes(api *mux.Router, m Mode) {
	api.HandleFunc("/myBalance", authRequire(m.getMyBalanceHandler)).Methods("GET")
	api.HandleFunc("/walletHistory", authRequire(getWalletHistory)).Methods("GET")
	api.HandleFunc("/tx_record/{hashes}", (getTxRecord)).Methods("GET")
}

//
func (m Mode) SetSubNodeRoutes(r Router) {
	api := r.GetAPIVersion("/api/v2")
	api.HandleFunc("/shareData/create", shareDataCreate).Methods("POST")
	api.HandleFunc("/shareData/update/{id}", shareDataUpdateHandlre).Methods("POST")
	api.HandleFunc("/shareData/delete/{id}", shareDataDeleteHandlre).Methods("POST")

	api.HandleFunc("/shareData/list", shareDataListHandlre).Methods("GET")
	api.HandleFunc("/shareData/{id}", shareDataByIDHandlre).Methods("GET")
	api.HandleFunc("/shareData/uuid/{taskuuid}", shareDataByTaskUUIDHandlre).Methods("GET")

	api.HandleFunc("/shareDataStatus/uuid/{taskuuid}", shareDataStatusByTaskUUIDHandlre).Methods("GET")
	api.HandleFunc("/privateData/list", privateDataListHandlre).Methods("GET")

	//
	api.HandleFunc("/SubNodeSrcTask/create", authRequire(SubNodeSrcTaskCreateHandlre)).Methods("POST")
	api.HandleFunc("/SubNodeSrcTask/update/{id}", authRequire(SubNodeSrcTaskUpdateHandlre)).Methods("POST")
	api.HandleFunc("/SubNodeSrcTask/delete/{id}", authRequire(SubNodeSrcTaskDeleteHandlre)).Methods("POST")
	api.HandleFunc("/SubNodeSrcTask/{id}", authRequire(SubNodeSrcTaskByIDHandlre)).Methods("GET")
	api.HandleFunc("/SubNodeSrcTask/uuid/{taskuuid}", authRequire(SubNodeSrcTaskByTaskUUIDHandlre)).Methods("GET")

	api.HandleFunc("/SubNodeSrcData/create", authRequire(SubNodeSrcDataCreateHandlre)).Methods("POST")
	//api.HandleFunc("/SubNodeSrcData/update/{id}", authRequire(SubNodeSrcDataUpdateHandlre)).Methods("POST")
	api.HandleFunc("/SubNodeSrcData/delete/{id}", authRequire(SubNodeSrcDataDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/SubNodeSrcData/list", authRequire(SubNodeSrcDataListHandlre)).Methods("GET")
	//api.HandleFunc("/SubNodeSrcData/{id}", authRequire(SubNodeSrcDataByIDHandlre)).Methods("GET")
	//api.HandleFunc("/SubNodeSrcData/uuid/{taskuuid}", authRequire(SubNodeSrcDataByTaskUUIDHandlre)).Methods("GET")

	//
	api.HandleFunc("/SubNodeListWhere/{name}", authRequire(getSubNodeListWhereHandler)).Methods("POST")
}

func (m Mode) SetVDESrcRoutes(r Router) {
	api := r.GetAPIVersion("/api/v2")

	api.HandleFunc("/VDESrcData/create", authRequire(VDESrcDataCreateHandlre)).Methods("POST")
	//api.HandleFunc("/VDESrcData/update/{id}", authRequire(VDESrcDataUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcData/delete/{id}", authRequire(VDESrcDataDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDESrcData/list", authRequire(VDESrcDataListHandlre)).Methods("GET")
	//api.HandleFunc("/VDESrcData/{id}", authRequire(VDESrcDataByIDHandlre)).Methods("GET")
	//api.HandleFunc("/VDESrcData/uuid/{taskuuid}", authRequire(VDESrcDataByTaskUUIDHandlre)).Methods("GET")

	api.HandleFunc("/VDESrcTask/create", authRequire(VDESrcTaskCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTask/update/{id}", authRequire(VDESrcTaskUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTask/delete/{id}", authRequire(VDESrcTaskDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDESrcTask/list", authRequire(VDESrcTaskListHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTask/{id}", authRequire(VDESrcTaskByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTask/uuid/{taskuuid}", authRequire(VDESrcTaskByTaskUUIDHandlre)).Methods("GET")

	//
	api.HandleFunc("/VDESrcTaskFromSche/create", authRequire(VDESrcTaskFromScheCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskFromSche/update/{id}", authRequire(VDESrcTaskFromScheUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskFromSche/delete/{id}", authRequire(VDESrcTaskFromScheDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskFromSche/list", authRequire(VDESrcTaskFromScheListHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTaskFromSche/{id}", authRequire(VDESrcTaskFromScheByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTaskFromSche/uuid/{taskuuid}", authRequire(VDESrcTaskFromScheByTaskUUIDHandlre)).Methods("GET")

	api.HandleFunc("/VDEScheTask/create", authRequire(VDEScheTaskCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheTask/update/{id}", authRequire(VDEScheTaskUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheTask/delete/{id}", authRequire(VDEScheTaskDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDEScheTask/list", authRequire(VDEScheTaskListHandlre)).Methods("GET")
	api.HandleFunc("/VDEScheTask/{id}", authRequire(VDEScheTaskByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDEScheTask/uuid/{taskuuid}", authRequire(VDEScheTaskByTaskUUIDHandlre)).Methods("GET")
	//api.HandleFunc("/VDEScheTask/create", VDEScheTaskCreateHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheTask/update/{id}", VDEScheTaskUpdateHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheTask/delete/{id}", VDEScheTaskDeleteHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheTask/list", VDEScheTaskListHandlre).Methods("GET")
	//api.HandleFunc("/VDEScheTask/{id}", VDEScheTaskByIDHandlre).Methods("GET")
	//api.HandleFunc("/VDEScheTask/uuid/{taskuuid}", VDEScheTaskByTaskUUIDHandlre).Methods("GET")

	api.HandleFunc("/VDESrcChainInfo/create", authRequire(VDESrcChainInfoCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcChainInfo/update/{id}", authRequire(VDESrcChainInfoUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcChainInfo/delete/{id}", authRequire(VDESrcChainInfoDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDESrcChainInfo/list", authRequire(VDESrcChainInfoListHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcChainInfo/{id}", authRequire(VDESrcChainInfoByIDHandlre)).Methods("GET")

	api.HandleFunc("/VDEScheChainInfo/create", authRequire(VDEScheChainInfoCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheChainInfo/update/{id}", authRequire(VDEScheChainInfoUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheChainInfo/delete/{id}", authRequire(VDEScheChainInfoDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDEScheChainInfo/list", authRequire(VDEScheChainInfoListHandlre)).Methods("GET")
	api.HandleFunc("/VDEScheChainInfo/{id}", authRequire(VDEScheChainInfoByIDHandlre)).Methods("GET")
	//api.HandleFunc("/VDEScheChainInfo/create", VDEScheChainInfoCreateHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheChainInfo/update/{id}", VDEScheChainInfoUpdateHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheChainInfo/delete/{id}", VDEScheChainInfoDeleteHandlre).Methods("POST")
	//api.HandleFunc("/VDEScheChainInfo/list", VDEScheChainInfoListHandlre).Methods("GET")
	//api.HandleFunc("/VDEScheChainInfo/{id}", VDEScheChainInfoByIDHandlre).Methods("GET")

	api.HandleFunc("/VDEDestChainInfo/create", authRequire(VDEDestChainInfoCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestChainInfo/update/{id}", authRequire(VDEDestChainInfoUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestChainInfo/delete/{id}", authRequire(VDEDestChainInfoDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDEDestChainInfo/list", authRequire(VDEDestChainInfoListHandlre)).Methods("GET")
	api.HandleFunc("/VDEDestChainInfo/{id}", authRequire(VDEDestChainInfoByIDHandlre)).Methods("GET")

	api.HandleFunc("/VDEDestDataStatus/create", authRequire(VDEDestDataStatusCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestDataStatus/update/{id}", authRequire(VDEDestDataStatusUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestDataStatus/delete/{id}", authRequire(VDEDestDataStatusDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestDataStatus/{id}", authRequire(VDEDestDataStatusByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDEDestDataStatus/list", authRequire(VDEDestDataStatusByTaskUUIDHandlre)).Methods("POST")

	api.HandleFunc("/VDEAgentChainInfo/create", authRequire(VDEAgentChainInfoCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEAgentChainInfo/update/{id}", authRequire(VDEAgentChainInfoUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEAgentChainInfo/delete/{id}", authRequire(VDEAgentChainInfoDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDEAgentChainInfo/list", authRequire(VDEAgentChainInfoListHandlre)).Methods("GET")
	api.HandleFunc("/VDEAgentChainInfo/{id}", authRequire(VDEAgentChainInfoByIDHandlre)).Methods("GET")

	api.HandleFunc("/VDESrcMember/create", authRequire(VDESrcMemberCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcMember/update/{id}", authRequire(VDESrcMemberUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcMember/delete/{id}", authRequire(VDESrcMemberDeleteHandlre)).Methods("POST")
	//api.HandleFunc("/VDESrcMember/list", authRequire(VDESrcMemberListHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcMember/{id}", authRequire(VDESrcMemberByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcMember/pubkey/{pubkey}", authRequire(VDESrcMemberByPubKeyHandlre)).Methods("GET")

	api.HandleFunc("/VDEScheMember/create", authRequire(VDEScheMemberCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheMember/update/{id}", authRequire(VDEScheMemberUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheMember/delete/{id}", authRequire(VDEScheMemberDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDEScheMember/{id}", authRequire(VDEScheMemberByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDEScheMember/pubkey/{pubkey}", authRequire(VDEScheMemberByPubKeyHandlre)).Methods("GET")

	api.HandleFunc("/VDESrcTaskAuth/create", authRequire(VDESrcTaskAuthCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskAuth/update/{id}", authRequire(VDESrcTaskAuthUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskAuth/delete/{id}", authRequire(VDESrcTaskAuthDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDESrcTaskAuth/{id}", authRequire(VDESrcTaskAuthByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTaskAuth/pubkey/{pubkey}", authRequire(VDESrcTaskAuthByPubKeyHandlre)).Methods("GET")
	api.HandleFunc("/VDESrcTaskAuth/uuid/{taskuuid}", authRequire(VDESrcTaskAuthByTaskUUIDHandlre)).Methods("GET")

	api.HandleFunc("/VDEAgentMember/create", authRequire(VDEAgentMemberCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEAgentMember/update/{id}", authRequire(VDEAgentMemberUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEAgentMember/delete/{id}", authRequire(VDEAgentMemberDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDEAgentMember/{id}", authRequire(VDEAgentMemberByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDEAgentMember/pubkey/{pubkey}", authRequire(VDEAgentMemberByPubKeyHandlre)).Methods("GET")

	api.HandleFunc("/VDEDestMember/create", authRequire(VDEDestMemberCreateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestMember/update/{id}", authRequire(VDEDestMemberUpdateHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestMember/delete/{id}", authRequire(VDEDestMemberDeleteHandlre)).Methods("POST")
	api.HandleFunc("/VDEDestMember/{id}", authRequire(VDEDestMemberByIDHandlre)).Methods("GET")
	api.HandleFunc("/VDEDestMember/pubkey/{pubkey}", authRequire(VDEDestMemberByPubKeyHandlre)).Methods("GET")

	api.HandleFunc("/listWhere/{name}", authRequire(getListWhereHandler)).Methods("POST")
	api.HandleFunc("/VDEListWhere/{name}", authRequire(getVDEListWhereHandler)).Methods("POST")
}

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
