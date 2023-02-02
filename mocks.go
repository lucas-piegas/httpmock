package httpmock

import (
	"github.com/httpmock/option"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Interactions struct {
	interactions map[string]*interactions
	lock         sync.RWMutex
	logger       *zap.Logger
}

type interactions struct {
	attempt          int
	requestResponses []RequestResponse
}

type RequestCaptureFunc func(capturedRequestBody []byte, capturedRequestHeaders http.Header)

type RequestResponse struct {
	Path                   string
	Method                 string
	ResponseHttpStatus     int
	ResponseObject         interface{}
	ResponseContentType    string
	CapturedRequestBody    []byte
	CapturedRequestHeaders http.Header
	DelayResponse          time.Duration
	RequestCaptureFunc     RequestCaptureFunc
}

func NewInteractions(logger *zap.Logger) *Interactions {
	if logger == nil {
		logger = zap.L()
	}

	mi := &Interactions{
		interactions: make(map[string]*interactions),
		lock:         sync.RWMutex{},
		logger:       logger,
	}
	mi.logger.Info("created new instance of Interactions")
	return mi
}

func NewRequestResponse(method string, path string, responseStatus int, responseObject interface{}, responseContentType string, requestCaptureFunc RequestCaptureFunc, opts option.HttpMockOptions) RequestResponse {
	req := RequestResponse{
		Path:                path,
		Method:              method,
		ResponseHttpStatus:  responseStatus,
		ResponseObject:      responseObject,
		ResponseContentType: responseContentType,
		RequestCaptureFunc:  requestCaptureFunc,
	}

	addDelay(&req, opts)
	return req
}

func (m *Interactions) Add(method string, path string, responseStatus int, responseObject interface{}, responseContentType string, requestCaptureFunc RequestCaptureFunc, opts ...option.HttpMockOptionFunc) *Interactions {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := getKey(method, path)
	mi, ok := m.interactions[key]
	if !ok {
		mi = &interactions{
			attempt:          0,
			requestResponses: make([]RequestResponse, 0, 10),
		}
		m.logger.Debug("adding interaction for key: " + key)
	}
	m.logger.Info("adding mock interaction", zap.String("method", method), zap.String("path", path), zap.Int("responseStatus", responseStatus))

	options := option.ProcessOptions(m.logger, opts)

	req := NewRequestResponse(method, path, responseStatus, responseObject, responseContentType, requestCaptureFunc, options)

	mi.requestResponses = append(mi.requestResponses, req)
	m.interactions[key] = mi

	return m
}

func (m *Interactions) NextInteraction(method string, path string) *RequestResponse {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := getKey(method, path)
	mi, ok := m.interactions[key]
	if !ok || mi.attempt >= len(mi.requestResponses) {
		m.logger.Warn("no interactions found for key: " + key)
		return nil
	}

	requestResponse := mi.requestResponses[mi.attempt]
	mi.attempt++
	return &requestResponse
}

func (m *Interactions) Interaction(method string, path string, attempt int) *RequestResponse {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := getKey(method, path)
	mi, ok := m.interactions[key]
	if !ok || attempt >= len(mi.requestResponses) {
		return nil
	}
	return &mi.requestResponses[attempt]
}

func (m *Interactions) AllInteractions(method string, path string) []RequestResponse {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := getKey(method, path)
	mi, ok := m.interactions[key]
	if !ok {
		return []RequestResponse{}
	}
	return mi.requestResponses
}

func (m *Interactions) Reset() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.interactions = make(map[string]*interactions)
}

func (r *RequestResponse) Capture(requestBody []byte, headers http.Header) {
	r.CapturedRequestBody = requestBody
	r.CapturedRequestHeaders = headers
	if r.RequestCaptureFunc != nil {
		r.RequestCaptureFunc(requestBody, headers)
	}
}

func getKey(method string, path string) string {
	return method + "_" + path
}

func addDelay(req *RequestResponse, options option.HttpMockOptions) {
	req.DelayResponse = options.Delay
}
