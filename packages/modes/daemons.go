type BlockchainDaemonsListsFactory struct{}

func (f BlockchainDaemonsListsFactory) GetDaemonsList() []string {
	return []string{
		"BlocksCollection",
		"BlockGenerator",
		"QueueParserTx",
		"QueueParserBlocks",
		"Disseminator",
		"Confirmations",
		"Scheduler",
		"ExternalNetwork",
	}
}

type OBSDaemonsListFactory struct{}

func (f OBSDaemonsListFactory) GetDaemonsList() []string {
	return []string{
		"Scheduler",
		"VDESrcDataStatus",
		"VDESrcDataStatusAgent",
		"VDEAgentData",
		"VDESrcData",
		"VDEScheTaskUpToChain",
		"VDEScheTaskUpToChainState",
		"VDESrcTaskUpToChain",
		"VDESrcTaskUpToChainState",
		"VDEDestTaskSrcGetFromChain",
		"VDEDestTaskScheGetFromChain",
		"VDESrcTaskScheGetFromChain",
		"VDEScheTaskInstallContractSrc",
		"VDEScheTaskInstallContractDest",
		"VDESrcTaskInstallContractSrc",
		"VDEDestTaskInstallContractDest",
		"VDEDestData",
		"VDEDestDataStatus",
		"VDESrcHashUpToChain",
		"VDESrcHashUpToChainState",
		"VDESrcLogUpToChain",
		"VDESrcLogUpToChainState",
		"VDEDestLogUpToChain",
		"VDEDestLogUpToChainState",
		"VDEDestDataHashGetFromChain",
		"VDESrcTaskStatus",
		"VDESrcTaskStatusRun",
		"VDESrcTaskStatusRunState",
		"VDESrcTaskFromScheStatus",
		"VDESrcTaskFromScheStatusRun",
		"VDESrcTaskFromScheStatusRunState",
		"VDEAgentLogUpToChain",
		"VDEScheTaskChainStatus",
		"VDEScheTaskChainStatusState",
		"VDESrcTaskChainStatus",
		"VDESrcTaskChainStatusState",
		"VDESrcTaskAuthChainStatus",
		"VDESrcTaskAuthChainStatusState",
		"VDEScheTaskSrcGetFromChain",
		"VDEScheTaskFromSrcInstallContractSrc",
		"VDEScheTaskFromSrcInstallContractDest",
	}
}

type SubNodeDaemonsListFactory struct{}

func (f SubNodeDaemonsListFactory) GetDaemonsList() []string {
	return []string{
		"BlocksCollection",
		"BlockGenerator",
		"QueueParserTx",
		"QueueParserBlocks",
		"Disseminator",
		"Confirmations",
		"Scheduler",
		"ShareTask",
		"SendPrivateData",
		"UpToChain",
		"CheckAllChainData",
		"SubNodeSrcTaskInstallChannel",
		"SubNodeSrcData",
		"SubNodeSrcDataStatus",
		"SubNodeSrcDataStatusAgent",
		"SubNodeAgentData",
		"SubNodeSrcDataUpToChain",
		"SubNodeSrcHashUpToChainState",
		"SubNodeDestData",
	}
}
