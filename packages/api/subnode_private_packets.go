	"net/http"

	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
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
