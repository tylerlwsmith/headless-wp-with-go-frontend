package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"webapp/wp"
)

type query map[string]any

type apiRequest[T any] struct {
	endpoint string
	query    query
}

func newRequest[T any](url string) *apiRequest[T] {
	return &apiRequest[T]{endpoint: url, query: make(map[string]any)}
}

func (req *apiRequest[T]) SetParam(key string, value any) *apiRequest[T] {
	if value != nil {
		req.query[key] = value
	} else {
		delete(req.query, key)
	}

	return req
}

func (q *query) toMap() map[string]string {
	params := make(map[string]string)

	for k, v := range *q {
		rt := reflect.TypeOf(v)
		switch rt.Kind() {
		case reflect.Slice:
			var strSlice []string
			sliceValue := reflect.ValueOf(v)

			for i := 0; i < sliceValue.Len(); i++ {
				subVal := sliceValue.Index(i)
				strSlice = append(strSlice, fmt.Sprint(subVal.Interface()))
			}

			params[k] = strings.Join(strSlice, ",")
		default:
			params[k] = fmt.Sprintf("%v", v)
		}
	}

	return params
}

func (req *apiRequest[T]) buildURL() (url *url.URL, err error) {
	url, err = url.Parse(req.endpoint)

	if err != nil {
		return nil, err
	}

	values := url.Query()

	for k, v := range req.query.toMap() {
		values.Set(k, v)
	}

	url.RawQuery = values.Encode()

	return url, err
}

func (req *apiRequest[T]) Get() (values *[]T, header http.Header, err error) {
	url, err := req.buildURL()

	if err != nil {
		return nil, header, err
	}

	header, err = unmarshalAPIRequest[*[]T](url.String(), &values)
	return values, header, err
}

func (req *apiRequest[T]) GetAll() (values *[]T, header http.Header, err error) {
	const maxAttempts = 4
	attempts := 0

START:
	values = &[]T{}
	total := 0
	currentPage := 1
	for allFetched := false; !allFetched; {
		var url *url.URL
		url, err = req.SetParam("page", currentPage).buildURL()
		if err != nil {
			return nil, header, err
		}

		requestValues := &[]T{}
		header, err = unmarshalAPIRequest[*[]T](url.String(), &requestValues)
		if err != nil {
			return nil, header, err
		}

		newTotal, err := strconv.Atoi(header.Get("X-WP-Total"))
		if err != nil {
			return nil, header, err
		}

		if total != 0 && total != newTotal {
			// This could be bug prone if posts were published every second,
			// but they aren't.
			if attempts < maxAttempts {
				attempts += 1
				goto START
			} else {
				err = errors.New("too many unsuccessful GetAll() attempts")
				return nil, header, err
			}
		} else {
			total = newTotal
			*values = append(*values, *requestValues...)
		}

		if len(*values) == total {
			allFetched = true
		} else {
			currentPage += 1
		}
	}

	return values, header, err
}

func unmarshalAPIRequest[T any](url string, value *T) (header http.Header, err error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode > 299 {
		return res.Header, errors.New("API returned non-200 status code")
	}

	bytes, err := io.ReadAll(res.Body)

	err = json.Unmarshal(bytes, &value)

	return res.Header, err
}

func Posts() (request *apiRequest[wp.WPPost]) {
	return newRequest[wp.WPPost]("http://wordpress:80/wp-json/wp/v2/posts")
}

func Tags() (request *apiRequest[wp.WPTag]) {
	return newRequest[wp.WPTag]("http://wordpress:80/wp-json/wp/v2/tags")
}
