package types

import log "github.com/sirupsen/logrus"

type EcosystemGetter interface {
	GetEcosystemLookup() ([]int64, []string, error)
	ValidateId(id, clientID int64, le *log.Entry) (int64, error)
	GetEcosystemName(id int64) (string, error)
}
