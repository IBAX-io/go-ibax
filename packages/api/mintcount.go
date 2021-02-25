package api

import (
		ret.ReturnFailureString(errInvalidWallet.Errorf(params["wallet"]).Error())
		JsonCodeResponse(w, &ret)
		return
	}
	if conf.Config.PoolPub.Enable {
		mc := &model.MintCount{}
		f, err := mc.Get(blockID)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting Key for wallet")
			ret.ReturnFailureString(err.Error())
			JsonCodeResponse(w, &ret)
			return
		}
		if f {
			ret.Return(mc, model.CodeSuccess)
			JsonCodeResponse(w, &ret)
			return
		} else {
			ret.ReturnFailureString("not find")
			JsonCodeResponse(w, &ret)
			return
		}
	} else {
		ret.ReturnFailureString("PoolPub.Enable false")
		JsonCodeResponse(w, &ret)
		return
	}
}
