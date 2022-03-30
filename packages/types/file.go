/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

//type File *Map

func NewFile() *Map {
	return LoadMap(map[string]any{
		"Name":     "",
		"MimeType": "",
		"Body":     []byte{},
	})
}

func NewFileFromMap(m map[string]any) (f *Map, ok bool) {
	var v any
	f = NewFile()

	if v, ok = m["Name"].(string); !ok {
		return
	}
	f.Set("Name", v)
	if v, ok = m["MimeType"].(string); !ok {
		return
	}
	f.Set("MimeType", v)
	if v, ok = m["Body"].([]byte); !ok {
		return
	}
	f.Set("Body", v)

	return
}
