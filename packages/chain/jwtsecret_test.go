/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestJWTSecretCreatedOnceAndKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "JWTSecret")
	first, err := loadJWTSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != jwtSecretSize || bytes.Equal(first, make([]byte, jwtSecretSize)) {
		t.Fatalf("secret %x", first)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode %04o", info.Mode().Perm())
	}
	again, err := loadJWTSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, again) {
		t.Fatal("the secret changed on the second start")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("%d files left in the keys directory", len(entries))
	}

	other, err := loadJWTSecret(filepath.Join(t.TempDir(), "JWTSecret"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, other) {
		t.Fatal("two nodes made the same secret")
	}
}

func TestJWTSecretRefusesInvalidFile(t *testing.T) {
	valid := strings.Repeat("ab", jwtSecretSize)
	for name, c := range map[string]struct {
		content string
		mode    os.FileMode
		want    string
	}{
		"empty":      {"", 0o600, "hex-encoded"},
		"short":      {valid[2:], 0o600, "hex-encoded"},
		"long":       {valid + "ab", 0o600, "hex-encoded"},
		"not hex":    {strings.Repeat("zz", jwtSecretSize), 0o600, "hex-encoded"},
		"group read": {valid, 0o640, "accessible to other users"},
		"world read": {valid, 0o604, "accessible to other users"},
	} {
		if runtime.GOOS == "windows" && c.mode != 0o600 {
			continue
		}
		path := filepath.Join(t.TempDir(), "JWTSecret")
		if err := os.WriteFile(path, []byte(c.content), c.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, c.mode); err != nil {
			t.Fatal(err)
		}
		if secret, err := loadJWTSecret(path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: returned %x, %v", name, secret, err)
		}
		if data, _ := os.ReadFile(path); string(data) != c.content {
			t.Errorf("%s: the file was rewritten", name)
		}
	}

	if _, err := loadJWTSecret(t.TempDir()); err == nil {
		t.Error("took a directory for the secret")
	}
	if _, err := loadJWTSecret(filepath.Join(t.TempDir(), "missing", "JWTSecret")); err == nil {
		t.Error("created a secret in a directory that does not exist")
	}
}

// A secret written by hand with a trailing newline is accepted
func TestJWTSecretTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "JWTSecret")
	if err := os.WriteFile(path, []byte(strings.Repeat("0f", jwtSecretSize)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := loadJWTSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, bytes.Repeat([]byte{0x0f}, jwtSecretSize)) {
		t.Fatalf("secret %x", secret)
	}
}
