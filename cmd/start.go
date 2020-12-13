/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/


// startCmd is starting node
var startCmd = &cobra.Command{
	Use:    "start",
	Short:  "Starting node",
	PreRun: loadConfigWKey,
	Run: func(cmd *cobra.Command, args []string) {
		daylight.Start()
	},
}

func init() {
	time.Local = time.UTC
	startCmd.Flags().BoolVar(&conf.Config.TestRollBack, "testRollBack", false, "Starts special set of daemons")
	startCmd.Flags().BoolVar(&conf.Config.FuncBench, "funcBench", false, "Disable access checking in some built-in functions for benchmarks")
}
