/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"

	log "github.com/sirupsen/logrus"
)

type ReqTypesFlag uint16

// Types of requests
const (
	RequestTypeHonorNode ReqTypesFlag = iota + 1
	RequestTypeNotHonorNode
	RequestTypeStopNetwork
	RequestTypeConfirmation
	RequestTypeBlockCollection
	RequestTypeMaxBlock
	RequestTypeSendPrivateData
	RequestTypeSendPrivateFile
	RequestTypeSendVDESrcData
	RequestTypeSendVDESrcDataAgent
	RequestTypeSendVDEAgentData
	RequestTypeSendSubNodeSrcData
	RequestTypeSendSubNodeSrcDataAgent
	RequestTypeSendSubNodeAgentData

	// BlocksPerRequest contains count of blocks per request
	//BlocksPerRequest int32 = 1000
	BlocksPerRequest int = 100

	MaxBlockSize = 10485760
)

var ErrNotAccepted = errors.New("Not accepted")
var ErrMaxSize = errors.New("Size greater than max size")

// SelfReaderWriter read from Reader to himself and write to io.Writer from himself
type SelfReaderWriter interface {
	Read(io.Reader) error
	Write(io.Writer) error
}

// RequestType is type of request
type RequestType struct {
	Type ReqTypesFlag
}

// Read read first 2 bytes to uint16
func (rt *RequestType) Read(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, &rt.Type)
}

func (rt *RequestType) Write(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, rt.Type)
}

// MaxBlockResponse is max block response
type MaxBlockResponse struct {
	BlockID int64
}

func (resp *MaxBlockResponse) Read(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, &resp.BlockID)
}

func (resp *MaxBlockResponse) Write(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, resp.BlockID)
}

// GetBodiesRequest contains BlockID
type GetBodiesRequest struct {
	BlockID      uint32
	ReverseOrder bool
}

func (req *GetBodiesRequest) Read(r io.Reader) error {
	if err := binary.Read(r, binary.LittleEndian, &req.BlockID); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading getBodiesRequest blockID")
		return err
	}

	order, err := readBool(r)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading GetBodiesRequest reverse order")
	}

	req.ReverseOrder = order
	return nil
}

func (req *GetBodiesRequest) Write(w io.Writer) error {

	if err := binary.Write(w, binary.LittleEndian, req.BlockID); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on sending GetBodiesRequest blockID")
		return err
	}

	if err := writeBool(w, req.ReverseOrder); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on sending GetBodiesRequest reverse order")
		return err
	}

	return nil
}

// GetBodyResponse is Data []bytes
type GetBodyResponse struct {
	Data []byte
}

func (resp *GetBodyResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Data = slice
	return nil
}

func (resp *GetBodyResponse) Write(w io.Writer) error {
	return writeSlice(w, resp.Data)
}

// ConfirmRequest contains request data
type ConfirmRequest struct {
	BlockID uint32
}

func (req *ConfirmRequest) Read(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, &req.BlockID)
}

func (req *ConfirmRequest) Write(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, req.BlockID)
}

// ConfirmResponse contains response data
type ConfirmResponse struct {
	// ConfType uint8
	Hash []byte `size:"32"`
}

func (resp *ConfirmResponse) Read(r io.Reader) error {
	h, err := readSliceWithSize(r, consts.HashSize)
	if err == io.EOF {
	} else if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading ConfirmResponse reverse order")
		return err
	}
	resp.Hash = h
	return nil
}

func (resp *ConfirmResponse) Write(w io.Writer) error {
	if err := writeSliceWithSize(w, resp.Hash, consts.HashSize); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on sending ConfiremResponse hash")
		return err
	}

	return nil
}

// DisRequest contains request data
type DisRequest struct {
	Data []byte
}

func (req *DisRequest) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading disseminator request")
		return err
	}

	req.Data = slice
	return nil
}

func (req *DisRequest) Write(w io.Writer) error {
	err := writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending disseminator request")
	}

	return err
}

// DisTrResponse contains response data
type DisTrResponse struct{}

// DisHashResponse contains response data
type DisHashResponse struct {
	Data []byte
}

func (resp *DisHashResponse) Read(r io.Reader) error {
	slice, err := ReadSliceWithMaxSize(r, uint64(syspar.GetMaxTxSize()))
	if err != nil {
		return err
	}

	resp.Data = slice
	return nil
}

func (resp *DisHashResponse) Write(w io.Writer) error {
	return writeSlice(w, resp.Data)
}

type StopNetworkRequest struct {
	Data []byte
}

func (req *StopNetworkRequest) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		return err
	}

	req.Data = slice
	return nil
}

func (req *StopNetworkRequest) Write(w io.Writer) error {
	return writeSlice(w, req.Data)
}

type StopNetworkResponse struct {
	Hash []byte
}

func (resp *StopNetworkResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		return err
	}

	resp.Hash = slice
	return nil
}

func (resp *StopNetworkResponse) Write(w io.Writer) error {
	return writeSlice(w, resp.Hash)
}

func readBool(r io.Reader) (bool, error) {
	var val uint8
	if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
		return false, err
	}

	return val > 0, nil
}

func writeBool(w io.Writer, val bool) error {
	var intVal int8
	if val {
		intVal = 1
	}

	return binary.Write(w, binary.LittleEndian, intVal)
}

func ReadSlice(r io.Reader) ([]byte, error) {
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, sizeBuf); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading bytes slice size")
		return nil, err
	}

	size, errInt := binary.Uvarint(sizeBuf)
	if errInt <= 0 {
		log.WithFields(log.Fields{"type": consts.ConversionError, "errInt": errInt}).Error("on convert sizeBuf to value")
		return nil, fmt.Errorf("wrong sizebuf")
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading block body")
		return nil, err
	}

	return data, nil
}

func ReadSliceWithMaxSize(r io.Reader, maxSize uint64) ([]byte, error) {
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, sizeBuf); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading bytes slice size")
		return nil, err
	}

	size, errInt := binary.Uvarint(sizeBuf)
	if errInt <= 0 {
		log.WithFields(log.Fields{"type": consts.ConversionError, "errInt": errInt}).Error("on convert sizeBuf to value")
		return nil, fmt.Errorf("wrong sizebuf")
	}

	if size > maxSize {
		return nil, ErrMaxSize
	}
		return nil, err
	}

	return data, nil
}

func readSliceToBuf(r io.Reader, buf []byte) ([]byte, error) {
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, sizeBuf); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading bytes slice size")
		return nil, err
	}

	size, errInt := binary.Uvarint(sizeBuf)
	if errInt <= 0 {
		log.WithFields(log.Fields{"type": consts.ConversionError, "errInt": errInt}).Error("on convirt sizeBuf to value")
		return nil, fmt.Errorf("wrong sizebuf")
	}

	if cap(buf) < int(size) {
		buf = make([]byte, size)
	}

	_, err := io.ReadFull(r, buf[:size])
	return buf, err
}

func writeSlice(w io.Writer, slice []byte) error {
	byteSize := make([]byte, 4)
	binary.PutUvarint(byteSize, uint64(len(slice)))

	w.Write(byteSize)
	_, err := w.Write(slice)
	return err
}

// if bytesLen < 0 then slice length reads before reading slice body
func readSliceWithSize(r io.Reader, size int) ([]byte, error) {
	var value int32
	slice := make([]byte, size)
	if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading integer from network")
		return slice, err
	}
	_, err := io.ReadFull(r, slice)
	return slice, err
}

func writeSliceWithSize(w io.Writer, value []byte, size int32) error {
	if err := binary.Write(w, binary.LittleEndian, size); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on writing size")
		return err
	}

	_, err := w.Write(value)
	return err
}
func SendRequestType(reqType int64, w io.Writer) error {
	_, err := w.Write(converter.DecToBin(reqType, 2))
	return err
}

func ReadInt(r io.Reader) (int64, error) {
	var value int64
	err := binary.Read(r, binary.LittleEndian, &value)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on reading integer from network")
		return 0, err
	}

	return value, nil
}

func WriteInt(value int64, w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, value); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on sending integer to network")
		return err
	}

	return nil
}

type PrivateDateRequest struct {
	Data []byte
}

func (req *PrivateDateRequest) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading disseminator request")
		return err
	}

	req.Data = slice
	return nil
}

func (req *PrivateDateRequest) Write(w io.Writer) error {
	err := writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending disseminator request")
	}

	return err
}

type PrivateDateResponse struct {
	Hash string
}

func (resp *PrivateDateResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *PrivateDateResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

type PrivateFileRequest struct {
	TaskUUID   string
	TaskName   string
	TaskSender string
	TaskType   string
	FileName   string
	MimeType   string
	Data       []byte
}

func (req *PrivateFileRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	TaskName_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskName request")
		return err
	}
	req.TaskName = string(TaskName_slice)

	TaskSender_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskSender request")
		return err
	}
	req.TaskSender = string(TaskSender_slice)

	TaskType_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskType request")
		return err
	}
	req.TaskType = string(TaskType_slice)

	FileName_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading FileName request")
		return err
	}
	req.FileName = string(FileName_slice)

	MimeType_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading MimeType request")
		return err
	}
	req.MimeType = string(MimeType_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *PrivateFileRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.TaskName))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskName request")
		return err
	}

	err = writeSlice(w, []byte(req.TaskSender))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskSender request")
		return err
	}

	err = writeSlice(w, []byte(req.TaskType))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskType request")
		return err
	}

	err = writeSlice(w, []byte(req.FileName))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending FileName request")
		return err
	}

	err = writeSlice(w, []byte(req.MimeType))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending MimeType request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type PrivateFileResponse struct {
	Hash string
}

func (resp *PrivateFileResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *PrivateFileResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

//

type SubNodeSrcDataRequest struct {
	TaskUUID           string
	DataUUID           string
	AgentMode          string
	TranMode           string
	DataInfo           string
	SubNodeSrcPubkey   string
	SubNodeAgentPubkey string
	SubNodeAgentIp     string
	SubNodeDestPubkey  string
	SubNodeDestIp      string
	Data               []byte
}

func (req *SubNodeSrcDataRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	////
	TranMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TranMode request")
		return err
	}
	req.TranMode = string(TranMode_slice)
	//

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	SubNodeSrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeSrcPubkey request")
		return err
	}
	req.SubNodeSrcPubkey = string(SubNodeSrcPubkey_slice)

	SubNodeAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentPubkey request")
		return err
	}
	req.SubNodeAgentPubkey = string(SubNodeAgentPubkey_slice)

	SubNodeAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentIp request")
		return err
	}
	req.SubNodeAgentIp = string(SubNodeAgentIp_slice)

	SubNodeDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestPubkey request")
		return err
	}
	req.SubNodeDestPubkey = string(SubNodeDestPubkey_slice)

	SubNodeDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestIp request")
		return err
	}
	req.SubNodeDestIp = string(SubNodeDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *SubNodeSrcDataRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}
	////
	err = writeSlice(w, []byte(req.TranMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TranMode request")
		return err
	}
	//

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeSrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeSrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.SubNodeDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type SubNodeSrcDataResponse struct {
	Hash string
}

func (resp *SubNodeSrcDataResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *SubNodeSrcDataResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

type SubNodeSrcDataAgentRequest struct {
	TaskUUID           string
	DataUUID           string
	AgentMode          string
	TranMode           string
	DataInfo           string
	SubNodeSrcPubkey   string
	SubNodeAgentPubkey string
	SubNodeAgentIp     string
	SubNodeDestPubkey  string
	SubNodeDestIp      string
	Data               []byte
}

func (req *SubNodeSrcDataAgentRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	TranMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TranMode request")
		return err
	}
	req.TranMode = string(TranMode_slice)

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	SubNodeSrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeSrcPubkey request")
		return err
	}
	req.SubNodeSrcPubkey = string(SubNodeSrcPubkey_slice)

	SubNodeAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentPubkey request")
		return err
	}
	req.SubNodeAgentPubkey = string(SubNodeAgentPubkey_slice)

	SubNodeAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentIp request")
		return err
	}
	req.SubNodeAgentIp = string(SubNodeAgentIp_slice)

	SubNodeDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestPubkey request")
		return err
	}
	req.SubNodeDestPubkey = string(SubNodeDestPubkey_slice)

	SubNodeDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestIp request")
		return err
	}
	req.SubNodeDestIp = string(SubNodeDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *SubNodeSrcDataAgentRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}

	err = writeSlice(w, []byte(req.TranMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TranMode request")
		return err
	}

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeSrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeSrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.SubNodeDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type SubNodeSrcDataAgentResponse struct {
	Hash string
}

func (resp *SubNodeSrcDataAgentResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *SubNodeSrcDataAgentResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

type SubNodeAgentDataRequest struct {
	TaskUUID           string
	DataUUID           string
	AgentMode          string
	TranMode           string
	DataInfo           string
	SubNodeSrcPubkey   string
	SubNodeAgentPubkey string
	SubNodeAgentIp     string
	SubNodeDestPubkey  string
	SubNodeDestIp      string
	Data               []byte
}

func (req *SubNodeAgentDataRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	TranMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TranMode request")
		return err
	}
	req.TranMode = string(TranMode_slice)

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	SubNodeSrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeSrcPubkey request")
		return err
	}
	req.SubNodeSrcPubkey = string(SubNodeSrcPubkey_slice)

	SubNodeAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentPubkey request")
		return err
	}
	req.SubNodeAgentPubkey = string(SubNodeAgentPubkey_slice)

	SubNodeAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeAgentIp request")
		return err
	}
	req.SubNodeAgentIp = string(SubNodeAgentIp_slice)

	SubNodeDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestPubkey request")
		return err
	}
	req.SubNodeDestPubkey = string(SubNodeDestPubkey_slice)

	SubNodeDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading SubNodeDestIp request")
		return err
	}
	req.SubNodeDestIp = string(SubNodeDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *SubNodeAgentDataRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}

	err = writeSlice(w, []byte(req.TranMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TranMode request")
		return err
	}

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeSrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeSrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.SubNodeDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.SubNodeDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending SubNodeDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type SubNodeAgentDataResponse struct {
	Hash string
}

func (resp *SubNodeAgentDataResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *SubNodeAgentDataResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

type VDESrcDataRequest struct {
	TaskUUID       string
	DataUUID       string
	AgentMode      string
	DataInfo       string
	VDESrcPubkey   string
	VDEAgentPubkey string
	VDEAgentIp     string
	VDEDestPubkey  string
	VDEDestIp      string
	Data           []byte
}

func (req *VDESrcDataRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	VDESrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDESrcPubkey request")
		return err
	}
	req.VDESrcPubkey = string(VDESrcPubkey_slice)

	VDEAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentPubkey request")
		return err
	}
	req.VDEAgentPubkey = string(VDEAgentPubkey_slice)

	VDEAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentIp request")
		return err
	}
	req.VDEAgentIp = string(VDEAgentIp_slice)

	VDEDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestPubkey request")
		return err
	}
	req.VDEDestPubkey = string(VDEDestPubkey_slice)

	VDEDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestIp request")
		return err
	}
	req.VDEDestIp = string(VDEDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *VDESrcDataRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.VDESrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDESrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.VDEDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type VDESrcDataResponse struct {
	Hash string
}

func (resp *VDESrcDataResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *VDESrcDataResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

//0305
type VDESrcDataAgentRequest struct {
	TaskUUID       string
	DataUUID       string
	AgentMode      string
	DataInfo       string
	VDESrcPubkey   string
	VDEAgentPubkey string
	VDEAgentIp     string
	VDEDestPubkey  string
	VDEDestIp      string
	Data           []byte
}

func (req *VDESrcDataAgentRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	VDESrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDESrcPubkey request")
		return err
	}
	req.VDESrcPubkey = string(VDESrcPubkey_slice)

	VDEAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentPubkey request")
		return err
	}
	req.VDEAgentPubkey = string(VDEAgentPubkey_slice)

	VDEAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentIp request")
		return err
	}
	req.VDEAgentIp = string(VDEAgentIp_slice)

	VDEDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestPubkey request")
		return err
	}
	req.VDEDestPubkey = string(VDEDestPubkey_slice)

	VDEDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestIp request")
		return err
	}
	req.VDEDestIp = string(VDEDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *VDESrcDataAgentRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.VDESrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDESrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.VDEDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type VDESrcDataAgentResponse struct {
	Hash string
}

func (resp *VDESrcDataAgentResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *VDESrcDataAgentResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}

//0306
type VDEAgentDataRequest struct {
	TaskUUID       string
	DataUUID       string
	AgentMode      string
	DataInfo       string
	VDESrcPubkey   string
	VDEAgentPubkey string
	VDEAgentIp     string
	VDEDestPubkey  string
	VDEDestIp      string
	Data           []byte
}

func (req *VDEAgentDataRequest) Read(r io.Reader) error {

	TaskUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading TaskUUID request")
		return err
	}
	req.TaskUUID = string(TaskUUID_slice)

	DataUUID_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataUUID request")
		return err
	}
	req.DataUUID = string(DataUUID_slice)

	AgentMode_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading AgentMode request")
		return err
	}
	req.AgentMode = string(AgentMode_slice)

	DataInfo_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading DataInfo request")
		return err
	}
	req.DataInfo = string(DataInfo_slice)

	VDESrcPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDESrcPubkey request")
		return err
	}
	req.VDESrcPubkey = string(VDESrcPubkey_slice)

	VDEAgentPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentPubkey request")
		return err
	}
	req.VDEAgentPubkey = string(VDEAgentPubkey_slice)

	VDEAgentIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEAgentIp request")
		return err
	}
	req.VDEAgentIp = string(VDEAgentIp_slice)

	VDEDestPubkey_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestPubkey request")
		return err
	}
	req.VDEDestPubkey = string(VDEDestPubkey_slice)

	VDEDestIp_slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading VDEDestIp request")
		return err
	}
	req.VDEDestIp = string(VDEDestIp_slice)

	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading Data request")
		return err
	}
	req.Data = slice

	return nil
}

func (req *VDEAgentDataRequest) Write(w io.Writer) error {

	var err error

	err = writeSlice(w, []byte(req.TaskUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending TaskUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.DataUUID))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataUUID request")
		return err
	}

	err = writeSlice(w, []byte(req.AgentMode))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending AgentMode request")
		return err
	}

	err = writeSlice(w, []byte(req.DataInfo))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending DataInfo request")
		return err
	}

	err = writeSlice(w, []byte(req.VDESrcPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDESrcPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentPubkey request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEAgentIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEAgentIp request")
		return err
	}

	err = writeSlice(w, []byte(req.VDEDestPubkey))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestPubkey request")
		return err
	}
	err = writeSlice(w, []byte(req.VDEDestIp))
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending VDEDestIp request")
		return err
	}

	//err = writeSlice(w, req.Data)
	err = writeSlice(w, req.Data)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on sending Data request")
		return err
	}
	return nil
}

type VDEAgentDataResponse struct {
	Hash string
}

func (resp *VDEAgentDataResponse) Read(r io.Reader) error {
	slice, err := ReadSlice(r)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on reading GetBodyResponse")
		return err
	}

	resp.Hash = string(slice)
	return nil
}

func (resp *VDEAgentDataResponse) Write(w io.Writer) error {
	return writeSlice(w, []byte(resp.Hash))
}
