/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

//type File *Map

func NewFile() *Map {
	return LoadMap(map[string]interface{}{
		"Name":     "",
		"MimeType": "",
		"Body":     []byte{},
	})
}

func NewFileFromMap(m map[string]interface{}) (f *Map, ok bool) {
	var v interface{}
	f = NewFile()

