package api

import (
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
				ExpiresAt:        dt,
				RefreshExpiresAt: dt,
			}
			gr.RefreshClaims()
		}
	}()
	// run
	select {}
}
