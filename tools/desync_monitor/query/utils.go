package query

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"
)

type ConcurrentMap struct {
	m  map[string]any
	mu sync.RWMutex
}

func (c *ConcurrentMap) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = value
}

func (c *ConcurrentMap) Get(key string) (bool, any) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res, ok := c.m[key]
	return ok, res
}

func sendGetRequest(url string, v any) error {
	resp, err := http.Get(url)
	if err != nil {
		log.WithFields(log.Fields{"url": url, "error": err}).Error("get requesting url")
		return err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("status code is not OK %d", resp.StatusCode)
		log.WithFields(log.Fields{"url": url, "error": err}).Error("incorrect status code")
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{"url": url, "error": err}).Error("reading response body")
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		log.WithFields(log.Fields{"data": string(data), "error": err}).Error("unmarshalling json to struct")
		return err
	}
	return nil
}
