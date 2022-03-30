package sqldb

import (
	"fmt"
	"net/http"
)

var (
	defaultStatus = http.StatusOK
	//ErrEcosystemNotFound = errors.New("Ecosystem not found")
	//errContract          = errType{"E_CONTRACT", "There is not %s contract", http.StatusNotFound}
	//errDBNil             = errType{"E_DBNIL", "DB is nil", defaultStatus}
	//errDeletedKey        = errType{"E_DELETEDKEY", "The key is deleted", http.StatusForbidden}
	//errEcosystem         = errType{"E_ECOSYSTEM", "Ecosystem %d doesn't exist", defaultStatus}
	//errEmptyPublic       = errType{"E_EMPTYPUBLIC", "Public key is undefined", http.StatusBadRequest}
	//errKeyNotFound       = errType{"E_KEYNOTFOUND", "Key has not been found", http.StatusNotFound}
	//errEmptySign         = errType{"E_EMPTYSIGN", "Signature is undefined", defaultStatus}
	//errHashWrong         = errType{"E_HASHWRONG", "Hash is incorrect", http.StatusBadRequest}
	//errHashNotFound      = errType{"E_HASHNOTFOUND", "Hash has not been found", defaultStatus}
	//errHeavyPage         = errType{"E_HEAVYPAGE", "This page is heavy", defaultStatus}
	//errInstalled         = errType{"E_INSTALLED", "Chain is already installed", defaultStatus}
	//errInvalidWallet     = errType{"E_INVALIDWALLET", "Wallet %s is not valid", http.StatusBadRequest}
	//errLimitForsign      = errType{"E_LIMITFORSIGN", "Length of forsign is too big (%d)", defaultStatus}
	//errLimitTxSize       = errType{"E_LIMITTXSIZE", "The size of tx is too big (%d)", defaultStatus}
	//errNotFound          = errType{"E_NOTFOUND", "Page not found", http.StatusNotFound}
	//errNotFoundRecord    = errType{"E_NOTFOUND", "Record not found", http.StatusNotFound}
	//errParamNotFound     = errType{"E_PARAMNOTFOUND", "Parameter %s has not been found", http.StatusNotFound}
	//errPermission        = errType{"E_PERMISSION", "Permission denied", http.StatusUnauthorized}
	//errQuery             = errType{"E_QUERY", "DB query is wrong", http.StatusInternalServerError}
	//errRecovered         = errType{"E_RECOVERED", "API recovered", http.StatusInternalServerError}
	//errServer            = errType{"E_SERVER", "Server error", defaultStatus}
	//errSignature         = errType{"E_SIGNATURE", "Signature is incorrect", http.StatusBadRequest}
	//errUnknownSign       = errType{"E_UNKNOWNSIGN", "Unknown signature", defaultStatus}
	//errStateLogin        = errType{"E_STATELOGIN", "%s is not a membership of ecosystem %s", http.StatusForbidden}
	//errTableNotFound     = errType{"E_TABLENOTFOUND", "Table %s has not been found", http.StatusNotFound}
	//errToken             = errType{"E_TOKEN", "Token is not valid", defaultStatus}
	//errTokenExpired      = errType{"E_TOKENEXPIRED", "Token is expired by %s", http.StatusUnauthorized}
	//errUnauthorized      = errType{"E_UNAUTHORIZED", "Unauthorized", http.StatusUnauthorized}
	//errUndefineval       = errType{"E_UNDEFINEVAL", "Value %s is undefined", defaultStatus}
	//errUnknownUID        = errType{"E_UNKNOWNUID", "Unknown uid", defaultStatus}
	//errCLB               = errType{"E_CLB", "Virtual Dedicated Ecosystem %d doesn't exist", defaultStatus}
	//errCLBCreated        = errType{"E_CLBCREATED", "Virtual Dedicated Ecosystem is already created", http.StatusBadRequest}
	//errRequestNotFound   = errType{"E_REQUESTNOTFOUND", "Request %s doesn't exist", defaultStatus}
	//errUpdating          = errType{"E_UPDATING", "Node is updating blockchain", http.StatusServiceUnavailable}
	//errStopping          = errType{"E_STOPPING", "Network is stopping", http.StatusServiceUnavailable}
	//errNotImplemented    = errType{"E_NOTIMPLEMENTED", "Not implemented", http.StatusNotImplemented}
	//errParamMoneyDigit   = errType{"E_PARAMMONEYDIGIT", "The number of decimal places cannot be exceeded ( %s )", http.StatusBadRequest}
	//errDiffKey           = CodeType{"E_DIFKEY", "Sender's key is different from tx key", defaultStatus}
	//errBannded           = errType{"E_BANNED", "The key is banned till %s", http.StatusForbidden}
	//errCheckRole         = errType{"E_CHECKROLE", "Access denied", http.StatusForbidden}
	//errNewUser           = errType{"E_NEWUSER", "Can't create a new user", http.StatusUnauthorized}
	CodeSystembusy = CodeType{-1, "System is busy", http.StatusOK, ""}
	CodeSuccess    = CodeType{0, "Success", http.StatusOK, "OK"}
	//CodeFileNotExists         = CodeType{40001, "File %s not exists", http.StatusOK, ""}
	//CodeFileFormatNotSupports = CodeType{40002, "File %s format is not supported", http.StatusOK, ""}
	CodeIlgmediafiletype    = CodeType{40003, "illegal media file type  ", http.StatusOK, ""}
	CodeIlgfiletype         = CodeType{40004, "illegal file type  ", http.StatusOK, ""}
	CodeFilesize            = CodeType{40005, "illegal file size  ", http.StatusOK, ""}
	CodeImagesize           = CodeType{40006, "illegal image file size  ", http.StatusOK, ""}
	CodeVoicesize           = CodeType{40007, "illegal voice file size  ", http.StatusOK, ""}
	CodeVideosize           = CodeType{40008, "illegal video file size  ", http.StatusOK, ""}
	CodeRequestformat       = CodeType{40009, "illegal request format  ", http.StatusOK, ""}
	CodeThumbnailfilesize   = CodeType{400010, "illegal thumbnail file size  ", http.StatusOK, ""}
	CodeUrllength           = CodeType{400011, "illegal URL length  ", http.StatusOK, ""}
	CodeMultimediafileempty = CodeType{400012, "The multimedia file is empty  ", http.StatusOK, ""}
	CodePostpacketempty     = CodeType{400013, "POST packet is empty ", http.StatusOK, ""}
	CodeContentempty        = CodeType{400014, "The content of the graphic message is empty. ", http.StatusOK, ""}
	CodeTextcmpty           = CodeType{400015, "text message content is empty ", http.StatusOK, ""}
	CodeMultimediasizelimit = CodeType{400016, "multimedia file size exceeds limit ", http.StatusOK, ""}
	CodeParamNotNull        = CodeType{400017, "Param  message content exceeds limit ", http.StatusOK, ""}
	CodeParamOutRange       = CodeType{400018, "Param out of range ", http.StatusOK, ""}
	CodeParam               = CodeType{400019, "Param error ", http.StatusOK, ""}
	CodeParamNotExists      = CodeType{400020, "Param is exists  ", http.StatusOK, ""}
	CodeParamType           = CodeType{400021, "Param type error ", http.StatusOK, ""}
	CodeParamKeyConflict    = CodeType{400022, "Param Keyword conflict error ", http.StatusOK, ""}
	CodeRecordExists        = CodeType{400023, "Record already exists  ", http.StatusOK, ""}
	CodeRecordNotExists     = CodeType{400024, "Record not exists error  ", http.StatusOK, ""}
	CodeNewRecordNotRelease = CodeType{400025, "New Record not Release error ", http.StatusOK, ""}
	CodeReleaseRule         = CodeType{400026, "Release rule error  ", http.StatusOK, ""}
	CodeDeleteRule          = CodeType{400027, "Delete Record  delete rule error  ", http.StatusOK, ""}
	CodeHelpDirNotExists    = CodeType{400028, "Help parentdir  not exists error  ", http.StatusOK, ""}

	CodeDBfinderr     = CodeType{400029, "DB find error   ", http.StatusOK, ""}
	CodeDBcreateerr   = CodeType{400030, "DB create error  ", http.StatusOK, ""}
	CodeDBupdateerr   = CodeType{400031, "DB update error  ", http.StatusOK, ""}
	CodeDBdeleteerr   = CodeType{400032, "DB delete error  ", http.StatusOK, ""}
	CodeDBopertionerr = CodeType{400033, "DB opertion error  ", http.StatusOK, ""}
	CodeJsonformaterr = CodeType{400034, "Json format error  ", http.StatusOK, ""}
	CodeBodyformaterr = CodeType{400035, "Body format error  ", http.StatusOK, ""}

	CodeFileNotExists = CodeType{400036, "File not exists", http.StatusOK, ""}
	//CodeFileFormatNotSupports = CodeType{40002, "File %s format is not supported", http.StatusOK, ""}
	CodeFileExists            = CodeType{400037, "File already exists", http.StatusOK, ""}
	CodeFileFormatNotSupports = CodeType{400038, "File format is not supported", http.StatusOK, ""}
	CodeFileCreated           = CodeType{400039, "Create File is not supported ", http.StatusOK, ""}
	CodeFileOpen              = CodeType{400039, "Open File is not supported", http.StatusOK, ""}
	CodeCheckParam            = CodeType{400040, "Param error: ", http.StatusOK, ""}
	CodeGenerateMine          = CodeType{400041, "new miner generate faile ", http.StatusOK, ""}
	CodeImportMine            = CodeType{400042, "import miner faile   ", http.StatusOK, ""}
	CodeBooltype              = CodeType{400043, "bool type error  ", http.StatusOK, ""}

	CodeUpdateRule               = CodeType{400044, "rule error  ", http.StatusOK, ""}
	CodePermissionDenied         = CodeType{400045, "Permission denied  ", http.StatusOK, ""}
	CodeNotMineDevidBindActiveid = CodeType{400046, "not mine devid boind Activeid  ", http.StatusOK, ""}
	CodeSignError                = CodeType{400047, "sign err ", http.StatusOK, ""}
	//CodeNotMineDevidBindActiveid = CodeType{400046, "not mine devid boind Activeid  ", http.StatusOK, ""}
	//CodeReleaseRule          = CodeType{400042, "Release rule  conflict %s ", http.StatusOK, ""}
	//CodeGenerateMine          = CodeType{400041, "new miner generate faile ", http.StatusOK, ""}
	//CodeGenerateMine          = CodeType{400041, "new miner generate faile ", http.StatusOK, ""}
)

type CodeType struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
	Msg     string `json:"msg"`
}

//
type errType struct {
	Err     string `json:"error"`
	Message string `json:"msg"`
	Status  int    `json:"-"`
}

func (et errType) Error() string {
	return et.Err
}

func (et errType) Errorf(v ...any) errType {
	et.Message = fmt.Sprintf(et.Message, v...)
	return et
}

func (ct CodeType) Errorf(err error) CodeType {
	et, ok := err.(errType)
	if !ok {
		et.Message = err.Error()
	}
	ct.Message = fmt.Sprintln(ct.Message, et.Message)
	ct.Msg = http.StatusText(ct.Status)
	return ct
}

func (ct CodeType) String(dat string) CodeType {
	ct.Message += " " + dat
	ct.Msg = http.StatusText(ct.Status)
	return ct
}

func (ct CodeType) Success() CodeType {
	ct.Msg = http.StatusText(ct.Status)
	return ct
}
