/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// jwtSecretSize is the size of the HMAC-SHA256 key API tokens are signed with
const jwtSecretSize = 32

// loadJWTSecret returns the secret stored in path, creating it with a fresh random one on the
// node's first start. Tokens stay valid across restarts because the secret does. A file that
// exists but is not a secret of this node is an error rather than replaced: replacing it would
// log every user out, and a file others can read lets them forge tokens of any account.
func loadJWTSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return createJWTSecret(path)
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("JWT secret %s is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("JWT secret %s is accessible to other users (mode %04o), it must be 0600", path, info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(secret) != jwtSecretSize {
		return nil, fmt.Errorf("JWT secret %s is not %d hex-encoded bytes; delete it to make a new one, which logs every user out", path, jwtSecretSize)
	}
	return secret, nil
}

// createJWTSecret writes a new secret to a temporary file and renames it into place, so a start
// that is interrupted leaves either no secret or a complete one
func createJWTSecret(path string) ([]byte, error) {
	secret := make([]byte, jwtSecretSize)
	if _, err := crand.Read(secret); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(hex.EncodeToString(secret)); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return secret, nil
}
