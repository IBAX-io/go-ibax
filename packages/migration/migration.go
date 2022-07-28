/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/migration/updates"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

const (
	eVer = `Wrong version %s`
)

var migrations = []*migration{
	// Initial schema
	{"0.0.1", migrationInitialTables, true},
	{"0.0.2", migrationInitialSchema, false},
}

var updateMigrations = []*migration{
	{"0.0.3", updates.MigrationUpdatePriceExec, false},
	{"0.0.4", updates.MigrationUpdateAccessExec, false},
	{"0.0.5", updates.MigrationUpdatePriceCreateExec, false},
}

type migration struct {
	version  string
	data     string
	template bool
}

type database interface {
	CurrentVersion() (string, error)
	ApplyMigration(string, string) error
}

func compareVer(a, b string) (int, error) {
	var (
		av, bv []string
		ai, bi int
		err    error
	)
	if av = strings.Split(a, `.`); len(av) != 3 {
		return 0, fmt.Errorf(eVer, a)
	}
	if bv = strings.Split(b, `.`); len(bv) != 3 {
		return 0, fmt.Errorf(eVer, b)
	}
	for i, v := range av {
		if ai, err = strconv.Atoi(v); err != nil {
			return 0, fmt.Errorf(eVer, a)
		}
		if bi, err = strconv.Atoi(bv[i]); err != nil {
			return 0, fmt.Errorf(eVer, b)
		}
		if ai < bi {
			return -1, nil
		}
		if ai > bi {
			return 1, nil
		}
	}
	return 0, nil
}

func migrate(db database, appVer string, migrations []*migration) error {
	dbVerString, err := db.CurrentVersion()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "err": err}).Errorf("parse version")
		return err
	}

	if cmp, err := compareVer(dbVerString, appVer); err != nil {
		log.WithFields(log.Fields{"type": consts.MigrationError, "err": err}).Errorf("parse version")
		return err
	} else if cmp >= 0 {
		return nil
	}

	for _, m := range migrations {
		if cmp, err := compareVer(dbVerString, m.version); err != nil {
			log.WithFields(log.Fields{"type": consts.MigrationError, "err": err}).Errorf("parse version")
			return err
		} else if cmp >= 0 {
			continue
		}
		if m.template {
			m.data, err = sqlConvert([]string{m.data})
			if err != nil {
				return err
			}
		}
		err = db.ApplyMigration(m.version, m.data)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "err": err, "version": m.version}).Errorf("apply migration")
			return err
		}

		log.WithFields(log.Fields{"version": m.version}).Debug("apply migration")
	}

	return nil
}

func runMigrations(db database, migrationList []*migration) error {
	return migrate(db, consts.VERSION, migrationList)
}

// InitMigrate applies initial migrations
func InitMigrate(db database) error {
	mig := migrations
	if conf.Config.IsSubNode() {
		//mig = append(mig, migrationsSub)
	}
	if conf.Config.IsSupportingCLB() {
		//mig = append(mig, migrationsCLB)
	}
	return runMigrations(db, mig)
}

// UpdateMigrate applies update migrations
func UpdateMigrate(db database) error {
	return runMigrations(db, updateMigrations)
}
