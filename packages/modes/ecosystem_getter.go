package modes

import (
	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

func GetEcosystemGetter() types.EcosystemGetter {
	if conf.Config.IsSupportingCLB() {
		return CLBEcosystemGetter{}
	}

	return BCEcosystemGetter{}
}

type BCEcosystemGetter struct {
	logger *log.Entry
}

func (ng BCEcosystemGetter) GetEcosystemName(id int64) (string, error) {
	ecosystem := &sqldb.Ecosystem{}
	found, err := ecosystem.Get(nil, id)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting ecosystem from db")
		return "", err
	}

	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound, "id": id, "error": api.ErrEcosystemNotFound}).Error("ecosystem not found")
		return "", err
	}

	return ecosystem.Name, nil
}

func (g BCEcosystemGetter) GetEcosystemLookup() ([]int64, []string, error) {
	return sqldb.GetAllSystemStatesIDs()
}

func (v BCEcosystemGetter) ValidateId(formEcosysID, clientEcosysID int64, le *log.Entry) (int64, error) {
	if formEcosysID <= 0 {
		return clientEcosysID, nil
	}

	count, err := sqldb.NewDbTransaction(nil).GetNextID("1_ecosystems")
	if err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting next id of ecosystems")
		return 0, err
	}

	if formEcosysID >= count {
		le.WithFields(log.Fields{"state_id": formEcosysID, "count": count, "type": consts.ParameterExceeded}).Error("ecosystem is larger then max count")
		return 0, api.ErrEcosystemNotFound
	}

	return formEcosysID, nil
}

type CLBEcosystemGetter struct{}

func (g CLBEcosystemGetter) GetEcosystemLookup() ([]int64, []string, error) {
	return []int64{1}, []string{"Platform ecosystem"}, nil
}

func (CLBEcosystemGetter) ValidateId(id, clientID int64, le *log.Entry) (int64, error) {
	return consts.DefaultCLB, nil
}

func (ng CLBEcosystemGetter) GetEcosystemName(id int64) (string, error) {
	return "Platform ecosystem", nil
}
