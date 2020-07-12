	return b.err.Error()
}

func WithBan(err error) error {
	return &BanError{
		err: err,
	}
}

func IsBanError(err error) bool {
	err = errors.Cause(err)
	if _, ok := err.(*BanError); ok {
		return true
	}
	return false
}
