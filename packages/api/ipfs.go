/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/gorilla/mux"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const leadingSlash = "/"

var (
	errPathPrefix = errType{"E_PathPrefix", "paths must start with a leading slash", http.StatusBadRequest}
	errPathsEmpty = errType{"E_PathsEmpty", "paths must not be empty", http.StatusBadRequest}
)

var TempDir = "./tempdir/"

type pathsForm struct {
	Paths string `schema:"paths"`
}

type sdForm struct {
	Source string `schema:"source"`
	Dest   string `schema:"dest"`
}
type addForm struct {
	pathsForm
	Key string `schema:"key"`
}
type nameForm struct {
	nopeValidator
	Name string `schema:"name"`
}
type gFResult struct {
	Hash map[string]string `json:"hash"`
}

type dirResult struct {
	Hash string `json:"hash"`
}

type statResult struct {
	Hash           string
	Size           uint64
	CumulativeSize uint64
	Blocks         int
	Type           string
	WithLocality   bool
	Local          bool
	SizeLocal      uint64
}

func (p *pathsForm) Validate(r *http.Request) error {
	v_path, err := checkPath(p.Paths)
	if err != nil {
		return err
	}
	p.Paths = v_path
	return nil
}

func (s *sdForm) Validate(r *http.Request) error {
	source, err := checkPath(s.Source)
	if err != nil {
		return err
	}
	dest, err := checkPath(s.Dest)
	if err != nil {
		return err
	}
	s.Source = source
	s.Dest = dest

	return nil
}

func filePre(w http.ResponseWriter, r *http.Request) {
	client := getClient(r)
	var form = new(nameForm)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	err := r.ParseMultipartForm(multipartBuf)
	if err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		errorResponse(w, errors.Wrapf(err, "getting current wd"), http.StatusBadRequest)
		return
	}

	dir := filepath.Join(cwd, "tempdir")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0775)
		if err != nil {
			errorResponse(w, errors.Wrapf(err, "creating dir %s", dir), http.StatusBadRequest)
			return
		}
	}
	result := &gFResult{Hash: make(map[string]string)}
	for key := range r.MultipartForm.File {
		err := getFileData(r, converter.
			Int64ToStr(client.KeyID), key)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Hash[key] = form.Name
	}
	jsonResponse(w, result)
}

func add(w http.ResponseWriter, r *http.Request) {
	var form = new(addForm)
	logger := getLogger(r)

	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result := &gFResult{Hash: make(map[string]string)}
	var res map[string]string
	if err := json.Unmarshal([]byte(form.Key), &res); err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling key")
		errorResponse(w, err)
		return
	}
	for key, name := range res {
		var fileData []byte
		var err error
		tempFile := TempDir + converter.Int64ToStr(client.KeyID) + key
		if fileData, err = os.ReadFile(tempFile); err != nil {
			logger.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading multipart file")
			errorResponse(w, err, http.StatusBadRequest)
			return
		}
		s := shell.NewShell(conf.IpfsHost())
		hash, err := s.Add(bytes.NewBuffer(fileData))
		if err != nil {
			errorResponse(w, err)
			return
		}
		respCp, err := s.Request("files/cp", fmt.Sprintf("/ipfs/%s", hash), leadingSlash+converter.
			Int64ToStr(client.KeyID)+form.Paths+name).
			Send(context.Background())
		if err != nil {
			errorResponse(w, err)
			return
		}
		defer respCp.Close()
		if respCp.Error != nil {
			errorResponse(w, respCp.Error)
			return
		}
		tempName := tempFile + ".temp"
		os.Remove(tempFile)
		os.Remove(tempName)
		result.Hash[key] = hash
	}
	jsonResponse(w, result)
}

func addDir(w http.ResponseWriter, r *http.Request) {
	form := &sdForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	s := shell.NewShell(conf.IpfsHost())

	hash, err := s.AddDir(form.Source)
	if err != nil {
		errorResponse(w, err)
		return
	}

	resp, err := s.Request("files/cp", fmt.Sprintf("/ipfs/%s", hash),
		leadingSlash+converter.Int64ToStr(client.KeyID)+form.Dest).
		Send(context.Background())
	if err != nil {
		errorResponse(w, err)
		return

	}
	defer resp.Close()
	if resp.Error != nil {
		errorResponse(w, resp.Error)
		return
	}
	jsonResponse(w, &dirResult{
		Hash: hash,
	})
}

func cat(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	type link struct {
		Link string
	}
	jsonResponse(w, link{Link: conf.IpfsHost() + fmt.Sprintf("/api/v0/cat?arg=/ipfs/%s", params["hash"])})
}

func filesMkdir(w http.ResponseWriter, r *http.Request) {
	form := &pathsForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	s := shell.NewShell(conf.IpfsHost())
	resp, err := s.Request("files/mkdir").
		Option("arg", leadingSlash+converter.Int64ToStr(client.KeyID)+form.Paths).
		Option("parents", true).
		Send(context.Background())
	if err != nil {
		errorResponse(w, err)
		return

	}
	defer resp.Close()
	if resp.Error != nil {
		errorResponse(w, resp.Error)
		return
	}
	jsonResponse(w, nil)
}

func filesStat(w http.ResponseWriter, r *http.Request) {
	form := &pathsForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	s := shell.NewShell(conf.IpfsHost())
	out := &statResult{}
	err := s.Request("files/stat").
		Option("arg", leadingSlash+converter.Int64ToStr(client.KeyID)+form.Paths).
		Exec(context.Background(), &out)
	if err != nil {
		errorResponse(w, err)
		return
	}

	jsonResponse(w, out)
}

func filesRm(w http.ResponseWriter, r *http.Request) {
	form := &pathsForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	if form.Paths == "/" {
		errorResponse(w, fmt.Errorf("cannot delete root"), http.StatusBadRequest)
		return
	}
	s := shell.NewShell(conf.IpfsHost())
	resp, err := s.Request("files/rm").
		Option("arg", leadingSlash+converter.Int64ToStr(client.KeyID)+form.Paths).
		Option("recursive", true).
		Send(context.Background())
	if err != nil {
		errorResponse(w, err)
		return

	}
	defer resp.Close()
	if resp.Error != nil {
		errorResponse(w, resp.Error)
		return
	}

	jsonResponse(w, nil)
}

func filesMv(w http.ResponseWriter, r *http.Request) {
	form := &sdForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	s := shell.NewShell(conf.IpfsHost())
	resp, err := s.Request("files/mv",
		leadingSlash+converter.Int64ToStr(client.KeyID)+form.Source,
		leadingSlash+converter.Int64ToStr(client.KeyID)+form.Dest).
		Send(context.Background())
	if err != nil {
		errorResponse(w, err)
		return

	}
	defer resp.Close()
	if resp.Error != nil {
		errorResponse(w, resp.Error)
		return
	}

	jsonResponse(w, nil)
}

func filesCp(w http.ResponseWriter, r *http.Request) {
	form := &pathsForm{}
	client := getClient(r)
	params := mux.Vars(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	s := shell.NewShell(conf.IpfsHost())

	resp, err := s.Request("files/cp", fmt.Sprintf("/ipfs/%s", params["hash"]),
		leadingSlash+converter.Int64ToStr(client.KeyID)+form.Paths).
		Send(context.Background())
	if err != nil {
		errorResponse(w, err)
		return

	}
	defer resp.Close()
	if resp.Error != nil {
		errorResponse(w, resp.Error)
		return
	}

	jsonResponse(w, nil)
}

func fileLs(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	s := shell.NewShell(conf.IpfsHost())
	list, err := s.FileList(fmt.Sprintf("/ipfs/%s", params["hash"]))
	if err != nil {
		errorResponse(w, err)
		return
	}
	jsonResponse(w, list)
}

func filesLs(w http.ResponseWriter, r *http.Request) {
	form := &pathsForm{}
	client := getClient(r)
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	s := shell.NewShell(conf.IpfsHost())
	var out interface{}
	err := s.Request("files/ls").Option("arg", leadingSlash+converter.Int64ToStr(client.KeyID)+form.Paths).Option("l", true).
		Exec(context.Background(), &out)
	if err != nil {
		errorResponse(w, err)
		return
	}
	jsonResponse(w, out)
}

func ls(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	s := shell.NewShell(conf.IpfsHost())
	list, err := s.List(fmt.Sprintf("/ipfs/%s", params["hash"]))
	if err != nil {
		errorResponse(w, err)
		return
	}
	jsonResponse(w, list)
}

func getFileData(r *http.Request, prefix, key string) error {
	logger := getLogger(r)

	fileByte, _, err := r.FormFile(key)
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("request.FormFile")
		return err
	}
	defer fileByte.Close()

	destName := TempDir + prefix + key
	//
	tempName := destName + ".temp"
	file2, _ := os.OpenFile(destName, os.O_WRONLY|os.O_CREATE, os.ModePerm) //
	file3, _ := os.OpenFile(tempName, os.O_RDWR|os.O_CREATE, os.ModePerm)   //
	defer file2.Close()
	defer file3.Close()

	//1.Read the amount of data stored in the last copy from the temporary file
	totalBytes := make([]byte, 100)
	count1, _ := file3.Read(totalBytes)     // Read the amount of data that has been copied into the array
	totalStr := string(totalBytes[:count1]) // Get the number of reads from the array，-->string
	total, _ := strconv.Atoi(totalStr)      //int
	toltemp := 0
	//2.
	fileByte.Seek(int64(toltemp), 0)
	file2.Seek(int64(total), 0)
	dataBytes := make([]byte, 1024)
	for {
		count2, err := fileByte.Read(dataBytes)
		if err == io.EOF {
			file3.Close()
			//os.Remove(tempName)
			break
		}
		file2.Write(dataBytes[:count2])
		total += count2
		toltemp += count2
		file3.Seek(0, 0)
		totalStr = strconv.Itoa(total)
		file3.WriteString(totalStr)
		//if total > 30000{
		// panic("")
		//}
	}

	return nil
}

func checkPath(p string) (string, error) {
	if len(p) == 0 {
		return "", errPathsEmpty
	}

	if p[0] != '/' {
		return "", errPathPrefix
	}

	cleaned := path.Clean(p)
	if p[len(p)-1] == '/' && p != "/" {
		cleaned += "/"
	}
	return cleaned, nil
}
