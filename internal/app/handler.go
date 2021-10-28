package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const (
	maxURLsNumber     = 20
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

type (
	urlsRequest struct {
		URLs []string `json:"urls"`
	}
	urlsResult struct {
		SourceURL string `json:"url"`
		Response  struct {
			StatusCode   int             `json:"code"`
			ResponseBody json.RawMessage `json:"body"`
		} `json:"response"`
	}
)

func (a *app) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Given condition: POST-method.
		if r.Method != http.MethodPost {
			invalidMethodErr := fmt.Errorf("method not allowed: expected %q: got %q", http.MethodPost, r.Method)
			writeResponse(w, invalidMethodErr, http.StatusMethodNotAllowed)
			log.Println("handler:", invalidMethodErr)
			return
		}

		// Given condition: JSON input.
		givenContentType := r.Header.Get(contentTypeHeader)
		if givenContentType != contentTypeJSON {
			invalidContentTypeErr := fmt.Errorf(
				`unsupported %q header: expected %q: got %q`,
				contentTypeHeader, contentTypeJSON, givenContentType)
			writeResponse(w, invalidContentTypeErr, http.StatusUnsupportedMediaType)
			log.Println("handler:", invalidContentTypeErr)
			return
		}

		if r.ContentLength == 0 {
			emptyContentErr := errors.New("bad request: empty request body")
			writeResponse(w, emptyContentErr, http.StatusBadRequest)
			log.Println("handler:", emptyContentErr)
			return
		}

		var jsonReq urlsRequest
		if err := json.NewDecoder(r.Body).Decode(&jsonReq); err != nil {
			var jsonErr error
			if ute, ok := err.(*json.UnmarshalTypeError); ok {
				jsonErr = fmt.Errorf("bad request: invalid type for %s: %v", ute.Value, ute.Type)
			} else {
				jsonErr = fmt.Errorf("bad request: %s", err.Error())
			}
			writeResponse(w, jsonErr, http.StatusBadRequest)
			log.Println("handler:", jsonErr)
			return
		}

		// Given condition: limited number of URLs. Handle edge cases.
		if len(jsonReq.URLs) == 0 {
			noURLsErr := errors.New("bad request: no URLs passed")
			writeResponse(w, noURLsErr, http.StatusBadRequest)
			log.Println("handler:", noURLsErr)
			return
		}
		if len(jsonReq.URLs) > maxURLsNumber {
			maxURLsNumberErr := fmt.Errorf(
				"max number of URLs exceeded: %d of %d",
				len(jsonReq.URLs), maxURLsNumber)
			writeResponse(w, maxURLsNumberErr, http.StatusBadRequest)
			log.Println("handler:", maxURLsNumberErr)
			return
		}

		// Given condition: get data from URLs or return first error.
		results, err := a.crawler.Crawl(r.Context(), jsonReq.URLs)
		if err != nil {
			writeResponse(w, err, http.StatusInternalServerError)
			log.Println("handler:", err)
			return
		}

		response := make([]urlsResult, len(results))
		for i, res := range results {
			response[i].SourceURL = res.SourceURL
			response[i].Response.StatusCode = res.StatusCode
			response[i].Response.ResponseBody = res.ResponseBody
		}

		writeResponse(w, response, http.StatusOK)
		return
	})
}

func writeResponse(w http.ResponseWriter, data interface{}, httpStatusCode int) {
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	w.WriteHeader(httpStatusCode)

	if err, isErr := data.(error); isErr {
		if _, err = w.Write([]byte(err.Error())); err != nil {
			log.Println("response: write data to buffer:", err)
		}
		return
	}

	resp := make(map[string]interface{})
	resp["results"] = data

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Println("response: marshal to json:", err)
	}
	if _, err = w.Write(jsonResp); err != nil {
		log.Println("response: write data to buffer:", err)
	}
}
