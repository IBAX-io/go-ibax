package jsonrpc

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

type NotSingle struct {
}

type dataApi struct {
}

func NewDataApi() *dataApi {
	return &dataApi{}
}

func compareHash(data []byte, urlHash string) bool {
	urlHash = strings.ToLower(urlHash)

	var hash []byte
	switch len(urlHash) {
	case 32:
		h := md5.Sum(data)
		hash = h[:]
	case 64:
		hash = crypto.Hash(data)
	}

	return hex.EncodeToString(hash) == urlHash
}

func (d *dataApi) BinaryVerify(ctx RequestContext, notSingle NotSingle, binaryId int64, hash string) *Error {
	if binaryId <= 0 {
		return DefaultError(fmt.Sprintf(invalidParams, "binary Id"))
	}
	if hash == "" {
		return DefaultError(fmt.Sprintf(invalidParams, "hash"))
	}
	r := ctx.HTTPRequest()
	w := ctx.HTTPResponseWriter()

	logger := getLogger(r)

	bin := sqldb.Binary{}
	found, err := bin.GetByID(binaryId)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Errorf("getting binary by id")
		return DefaultError(err.Error())
	}

	if !found {
		return NotFoundError()
	}

	if !compareHash(bin.Data, hash) {
		return DefaultError("Hash is incorrect")
	}

	w.Header().Set("Content-Type", bin.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, bin.Name))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(bin.Data)
	return nil
}

func (d *dataApi) DataVerify(ctx RequestContext, notSingle NotSingle, table, column string, id int64, hash string) *Error {
	if table == "" || column == "" || id <= 0 || hash == "" {
		return InvalidParamsError("tableName or column or id or hash invalid")
	}
	r := ctx.HTTPRequest()
	w := ctx.HTTPResponseWriter()
	logger := getLogger(r)

	data, err := sqldb.GetColumnByID(table, column, id)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting data from table")
		return NotFoundError()
	}

	if !compareHash([]byte(data), hash) {
		return DefaultError("Hash is incorrect")
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(data))
	return nil
}

func (d *dataApi) GetAvatar(ctx RequestContext, notSingle NotSingle, account string, ecosystemId int64) *Error {
	if account == "" || ecosystemId <= 0 {
		return InvalidParamsError("account or ecosystemId invalid")
	}
	r := ctx.HTTPRequest()
	w := ctx.HTTPResponseWriter()
	logger := getLogger(r)

	member := &sqldb.Member{}
	member.SetTablePrefix(converter.Int64ToStr(ecosystemId))

	found, err := member.Get(account)
	if err != nil {
		logger.WithFields(log.Fields{
			"type":      consts.DBError,
			"error":     err,
			"ecosystem": ecosystemId,
			"account":   account,
		}).Error("getting member")
		return DefaultError(err.Error())
	}

	if !found {
		return NotFoundError()
	}

	if member.ImageID == nil {
		return NotFoundError()
	}

	bin := &sqldb.Binary{}
	bin.SetTablePrefix(converter.Int64ToStr(ecosystemId))
	found, err = bin.GetByID(*member.ImageID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "image_id": *member.ImageID}).Errorf("on getting binary by id")

		return DefaultError(err.Error())
	}

	if !found {
		return NotFoundError()
	}

	if len(bin.Data) == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject, "error": err, "image_id": *member.ImageID}).Errorf("on check avatar size")
		return NotFoundError()
	}

	w.Header().Set("Content-Type", bin.MimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(bin.Data)))
	if _, err := w.Write(bin.Data); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("unable to write image")
	}
	return nil
}
