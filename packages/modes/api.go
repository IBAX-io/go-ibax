
	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/conf"
)

func RegisterRoutes() http.Handler {
	m := api.Mode{
		EcosysIDValidator:  GetEcosystemIDValidator(),
		EcosysNameGetter:   BuildEcosystemNameGetter(),
		EcosysLookupGetter: BuildEcosystemLookupGetter(),
		ContractRunner:     GetSmartContractRunner(),
		ClientTxProcessor:  GetClientTxPreprocessor(),
	}

	r := api.NewRouter(m)
	if !conf.Config.IsSupportingOBS() {
		m.SetBlockchainRoutes(r)
	}
	if conf.GetGFiles() {
		m.SetGafsRoutes(r)
	}
	if conf.Config.IsSubNode() {
		m.SetSubNodeRoutes(r)
	}

	//0303
	if conf.Config.IsSupportingOBS() {
		m.SetVDESrcRoutes(r)
	}

	return r.GetAPI()
}
