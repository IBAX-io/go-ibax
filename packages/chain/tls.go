/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import "crypto/tls"

// serverTLSConfig follows NIST SP 800-52 Rev. 2: TLS 1.3, or TLS 1.2 with ECDHE key exchange and
// AES-GCM only. The TLS 1.3 suites are all AEAD and not configurable; in FIPS 140-3 mode Go
// further limits both versions to approved suites, groups and signature schemes.
func serverTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		SessionTicketsDisabled: true,
	}
}
