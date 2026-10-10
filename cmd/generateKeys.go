/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const fileMode = 0600

var accountAlgo string

// generateKeysCmd represents the generateKeys command
var generateKeysCmd = &cobra.Command{
	Use:    "generateKeys",
	Short:  "Keys generation",
	PreRun: loadConfig,
	Run: func(cmd *cobra.Command, args []string) {
		algo := crypto.NodeAlgo()
		if accountAlgo != "" {
			v, ok := crypto.AsymAlgo_value[accountAlgo]
			if !ok || !crypto.AccountAlgoImplemented(crypto.AsymAlgo(v)) {
				log.WithFields(log.Fields{"algo": accountAlgo}).Fatal("the account algorithm is not implemented")
			}
			algo = crypto.AsymAlgo(v)
		}
		accountKey, err := createAccountKey(algo,
			filepath.Join(conf.Config.DirPathConf.KeysDir, consts.PrivateKeyFilename),
			filepath.Join(conf.Config.DirPathConf.KeysDir, consts.PublicKeyFilename),
		)
		if err != nil {
			log.WithError(err).Fatal("generating user keys")
			return
		}
		_, _, err = createKeyPair(
			filepath.Join(conf.Config.DirPathConf.KeysDir, consts.NodePrivateKeyFilename),
			filepath.Join(conf.Config.DirPathConf.KeysDir, consts.NodePublicKeyFilename),
		)
		if err != nil {
			log.WithError(err).Fatal("generating node keys")
			return
		}
		address := accountKey.Address()
		keyIDPath := filepath.Join(conf.Config.DirPathConf.KeysDir, consts.KeyIDFilename)
		err = createFile(keyIDPath, []byte(strconv.FormatInt(address, 10)))
		if err != nil {
			log.WithFields(log.Fields{"error": err, "path": keyIDPath}).Fatal("generating node keys")
			return
		}
		log.Info("keys generated")
	},
}

func init() {
	generateKeysCmd.Flags().StringVar(&accountAlgo, "accountAlgo", "", "algorithm of the account key, e.g. MLDSA65, ECC_P384 or RSA3072 (default: the node algorithm); an RSA private key is PKCS #1 DER")
}

// createAccountKey writes an account private key and its account public key, which carries its
// algorithm (crypto.AccountKey)
func createAccountKey(algo crypto.AsymAlgo, privFilename, pubFilename string) (crypto.AccountKey, error) {
	priv, key, err := crypto.GenAccountKey(algo)
	if err != nil {
		return key, err
	}
	if err = createFile(privFilename, []byte(hex.EncodeToString(priv))); err != nil {
		return key, err
	}
	return key, createFile(pubFilename, []byte(key.Hex()))
}

func createFile(filename string, data []byte) error {
	dir := filepath.Dir(filename)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0775)
		if err != nil {
			return errors.Wrapf(err, "creating dir %s", dir)
		}
	}

	return os.WriteFile(filename, data, fileMode)
}

func createKeyPair(privFilename, pubFilename string) (priv, pub []byte, err error) {
	priv, pub, err = crypto.GenNodeKeyPair()
	if err != nil {
		log.WithError(err).Error("generate keys")
		return
	}

	err = createFile(privFilename, []byte(hex.EncodeToString(priv)))
	if err != nil {
		log.WithFields(log.Fields{"error": err, "path": privFilename}).Error("creating private key")
		return
	}

	err = createFile(pubFilename, []byte(crypto.PubToHex(pub)))
	if err != nil {
		log.WithFields(log.Fields{"error": err, "path": pubFilename}).Error("creating public key")
		return
	}
	return
}
