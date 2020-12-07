package api

import (
	"sync"
	"time"
)

type GRefreshClaims struct {
	Header           string `json:"aud,omitempty"`
	Refresh          string `json:"ref,omitempty"`
	ExpiresAt        int64  `json:"exp,omitempty"`
	RefreshExpiresAt int64  `json:"refexp,omitempty"`
}

type GRefreshClaimsCache struct {
	mutex sync.RWMutex
	cache map[string]*GRefreshClaims
}

var GClaims = &GRefreshClaimsCache{cache: make(map[string]*GRefreshClaims)}

func (g *GRefreshClaims) ContainsClaims(h string) bool {

	if len(GClaims.cache) == 0 {
		return false
	}

	GClaims.mutex.RLock()
	defer GClaims.mutex.RUnlock()
	// key exist
	if v, ok := GClaims.cache[h]; ok {
		//return true
		ts := time.Now().Unix()
		if v.RefreshExpiresAt > ts {
			*g = *v
			return true
		}
	} else {
		return false
	}
	return false
}

func (g *GRefreshClaims) RefreshClaims() {

	if len(GClaims.cache) == 0 {
		GClaims.cache = make(map[string]*GRefreshClaims)
	}

	GClaims.mutex.Lock()
	defer GClaims.mutex.Unlock()

	GClaims.cache[g.Header] = g
}
