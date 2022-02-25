/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package statsd

import (
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf"

	"github.com/cactus/go-statsd-client/v5/statsd"
)

const (
	Count = ".count"
	Time  = ".time"
)

var Client statsd.Statter

func Init(conf conf.StatsDConfig) error {
	var err error
	config := &statsd.ClientConfig{
		Address:     fmt.Sprintf("%s:%d", conf.Host, conf.Port),
		Prefix:      conf.Name,
		UseBuffered: false,
	}
	Client, err = statsd.NewClientWithConfig(config)
	if err != nil {
		return err
	}
	return nil
}

func Close() {
	if Client != nil {
		Client.Close()
	}
}

func APIRouteCounterName(method, pattern string) string {
	routeCounterName := strings.Replace(strings.Replace(pattern, ":", "", -1), "/", ".", -1)
	return "api." + strings.ToLower(method) + "." + routeCounterName
}

func DaemonCounterName(daemonName string) string {
	return "daemon." + daemonName
}
