/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package statsd

import (
	"fmt"
	"strings"

	"github.com/cactus/go-statsd-client/v5/statsd"
)

const (
	Count = ".count"
	Time  = ".time"
)

var Client statsd.Statter

func Init(host string, port int, name string) error {
	var err error
	Client, err = statsd.NewClient(fmt.Sprintf("%s:%d", host, port), name)

func DaemonCounterName(daemonName string) string {
	return "daemon." + daemonName
}
