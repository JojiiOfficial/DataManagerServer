package handlers

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/DataManager-Go/DataManagerServer/models"

	"github.com/JojiiOfficial/gaw"
	log "github.com/sirupsen/logrus"
)

func readRequestLimited(w http.ResponseWriter, r *http.Request, p interface{}, limit int64) bool {
	return readRequestBody(w, io.LimitReader(r.Body, limit), p)
}

// Read the request body
func readRequestBody(w http.ResponseWriter, r io.Reader, p interface{}) bool {
	body, err := ioutil.ReadAll(r)

	if LogError(err) {
		return false
	}

	return !handleAndSendError(json.Unmarshal(body, p), w, WrongInputFormatError, http.StatusUnprocessableEntity)
}

// LogError returns true on error
func LogError(err error, context ...map[string]interface{}) bool {
	if err == nil {
		return false
	}

	if len(context) > 0 {
		log.WithFields(context[0]).Error(err.Error())
	} else {
		log.Error(err.Error())
	}
	return true
}

// AllowedSchemes schemes that are allowed in urls
var AllowedSchemes = []string{"http", "https"}

func isValidHTTPURL(inp string) bool {
	//check for valid URL
	u, err := url.Parse(inp)
	if err != nil {
		return false
	}

	return gaw.IsInStringArray(u.Scheme, AllowedSchemes)
}

func isStructInvalid(x interface{}) bool {
	s := reflect.TypeOf(x)
	for i := s.NumField() - 1; i >= 0; i-- {
		e := reflect.ValueOf(x).Field(i)

		if hasEmptyValue(e) {
			return true
		}
	}
	return false
}

func hasEmptyValue(e reflect.Value) bool {
	switch e.Type().Kind() {
	case reflect.String:
		if e.String() == "" || strings.Trim(e.String(), " ") == "" {
			return true
		}
	case reflect.Array:
		for j := e.Len() - 1; j >= 0; j-- {
			isEmpty := hasEmptyValue(e.Index(j))
			if isEmpty {
				return true
			}
		}
	case reflect.Slice:
		return isStructInvalid(e)

	case
		reflect.Uintptr, reflect.Ptr, reflect.UnsafePointer,
		reflect.Uint64, reflect.Uint, reflect.Uint8, reflect.Bool,
		reflect.Struct, reflect.Int64, reflect.Int:
		{
			return false
		}
	default:
		log.Error(e.Type().Kind(), e)
		return true
	}
	return false
}

func downloadHTTP(user *models.User, url string, f *os.File, file *models.File) (int, error) {
	if !isValidHTTPURL(url) {
		return 0, errors.New("invalid url")
	}

	httpClient := http.Client{
		Timeout: 10 * time.Second,
	}

	res, err := httpClient.Get(url)
	if LogError(err) {
		return 0, err
	}

	// Don't read content on http error
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, nil
	}

	// Check if file is too large
	if user.HasUploadLimit() && res.ContentLength > user.Role.MaxURLcontentSize {
		return res.StatusCode, errors.New("File too large")
	}

	// Read response
	var reader io.Reader
	if user.HasUploadLimit() {
		// Use limited reader if user has limited download content size
		reader = io.LimitReader(res.Body, user.Role.MaxURLcontentSize)
	} else {
		// use body as reader to read everything
		reader = res.Body
	}

	hash := crc32.NewIEEE()

	// Save body in file
	size, err := io.Copy(io.MultiWriter(f, hash), reader)
	if LogError(err) {
		return 0, err
	}

	if err = res.Body.Close(); LogError(err) {
		return 0, err
	}

	// Set file size and checksum
	file.FileSize = size
	file.Checksum = hex.EncodeToString(hash.Sum(nil))
	return res.StatusCode, nil
}

var (
	// ErrMissingFile error if file is missing
	ErrMissingFile = errors.New("missing upload file")
)

const (
	bufferSize = 1024 * 1024
	boundary   = "MachliJalKiRaniHaiJeevanUskaPaaniHai"
)

// Just a little magic, nothing to see here
func readMultipartToFile(f *os.File, reader io.Reader, w http.ResponseWriter) (int64, string, error) {
	partReader := multipart.NewReader(reader, boundary)
	// just use first part
	part, err := partReader.NextPart()
	if err == io.EOF {
		return 0, "", ErrMissingFile
	}

	var size int64
	var n int
	// Crc hash to verify upload file
	hash := crc32.NewIEEE()
	// multiwriter for file and hash
	writer := io.MultiWriter(f, hash)
	// Buffer reading
	buf := make([]byte, bufferSize)

	for {
		n, err = part.Read(buf)
		if err != nil && err != io.EOF {
			return 0, "", err
		}

		if n > 0 {
			size += int64(n)

			// Write file and hash
			// Don't tolerate any error
			_, err := writer.Write(buf[:n])
			if err != nil {
				return 0, "", err
			}
		}

		// If error is EOF uploading
		// is done
		if err == io.EOF {
			break
		}
	}

	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
