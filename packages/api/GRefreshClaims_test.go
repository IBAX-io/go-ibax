package api

import (
	"testing"
	"time"
)

func TestMapRefresh(t *testing.T) {
	//assert.NoError(t, keyLogin(1))

	// start
	go func() {
		// run
		for {
			dt := time.Now().Unix()
			gr := GRefreshClaims{
				Header:           "abc",
				Refresh:          "cd",
				ExpiresAt:        dt,
				RefreshExpiresAt: dt,
			}
			gr.RefreshClaims()
		}
	}()

	go func() {
		// run
		for {
			dt := time.Now().Unix()
			gr := GRefreshClaims{
				Header:           "abc",
				Refresh:          "cd",
