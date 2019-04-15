/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd
	Short:  "Initializing database",
	PreRun: loadConfigWKey,
	Run: func(cmd *cobra.Command, args []string) {
		if err := model.InitDB(conf.Config.DB); err != nil {
			log.WithError(err).Fatal("init db")
		}
		log.Info("initDatabase completed")
	},
}
