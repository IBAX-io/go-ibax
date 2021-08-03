/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import "github.com/IBAX-io/go-ibax/packages/model"

type intervalBlocksCounter interface {
	count(state blockGenerationState) (int, error)
