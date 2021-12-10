/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptyGetBodyResponse(t *testing.T) {
	buf := []byte{}
	w := bytes.NewBuffer(buf)
	empty := &GetBodyResponse{}
	require.NoError(t, empty.Write(w))

	r := bytes.NewReader(w.Bytes())
	emptyRes := &GetBodyResponse{}
	require.NoError(t, emptyRes.Read(r))
}

func TestWriteReadInts(t *testing.T) {
	buf := []byte{}
	b := bytes.NewBuffer(buf)
	st := uint16(2)
	require.NoError(t, binary.Write(b, binary.LittleEndian, st))

	var val uint16
	err := binary.Read(b, binary.LittleEndian, &val)
	require.NoError(t, err)
	require.Equal(t, val, st)
	fmt.Println(val)
}

func TestRequestType(t *testing.T) {
	rt := RequestType{1}
	buf := []byte{}
	b := bytes.NewBuffer(buf)

	result := RequestType{}
	require.NoError(t, rt.Write(b))
	require.NoError(t, result.Read(b))
	require.Equal(t, rt, result)
	fmt.Println(rt, result)

}

func TestGetBodyResponse(t *testing.T) {
	rt := GetBodyResponse{Data: make([]byte, 4, 4)}
	buf := []byte{}
	b := bytes.NewBuffer(buf)

	result := GetBodyResponse{}
	require.NoError(t, rt.Write(b))
	require.NoError(t, result.Read(b))
	require.Equal(t, rt, result)
	fmt.Println(rt, result)

}

func TestBodyResponse(t *testing.T) {
	rt := GetBodyResponse{Data: []byte(strings.Repeat("A", 32))}
	buf := []byte{}
	b := bytes.NewBuffer(buf)

	result := &GetBodyResponse{}
	require.NoError(t, rt.Write(b))
	require.NoError(t, result.Read(b))
	require.Equal(t, rt.Data, result.Data)
	fmt.Println(rt, result)

}
