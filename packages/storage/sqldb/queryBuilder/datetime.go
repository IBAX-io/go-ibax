/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"errors"
	"regexp"
	"strings"
)

var (
	dateTimeLiteralRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?)?$`)
	timeLiteralRE     = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?$`)
	unixTimeRE        = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

	// ErrDateTimeValue is the error of a date or time value that is not a literal, such as 'now' or
	// 'today' which the database would read as the time of the node executing the statement
	ErrDateTimeValue = errors.New(`a date or time value must be YYYY-MM-DD[ HH:MM[:SS[.ffffff]]] for a date or timestamp, HH:MM[:SS[.ffffff]] for a time, or the seconds since the epoch for a timestamp field`)
)

// checkDateTimeValue rejects a value of a column of the data type that is not a date or time literal.
// A column that is not a date or time column accepts any value
func checkDateTimeValue(dataType, value string) error {
	var re *regexp.Regexp
	switch {
	case dataType == "date" || strings.HasPrefix(dataType, "timestamp"):
		re = dateTimeLiteralRE
	case strings.HasPrefix(dataType, "time"):
		re = timeLiteralRE
	default:
		return nil
	}
	if !re.MatchString(value) {
		return ErrDateTimeValue
	}
	return nil
}

// checkUnixTime rejects a value of a timestamp field that is not the seconds since the epoch
func checkUnixTime(value string) error {
	if !unixTimeRE.MatchString(value) {
		return ErrDateTimeValue
	}
	return nil
}
