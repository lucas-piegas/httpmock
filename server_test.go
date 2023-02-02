package httpmock

import (
	"fmt"
	"github.com/httpmock/option"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func TestMockServer_AddInteraction(t *testing.T) {
	type args struct {
		method              string
		path                string
		responseStatus      int
		responseObject      interface{}
		responseContentType string
		delay               time.Duration
		requestCaptureFunc  RequestCaptureFunc
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "get method",
			args: args{
				method:              http.MethodGet,
				path:                "/",
				responseStatus:      http.StatusOK,
				responseObject:      nil,
				responseContentType: "JSON",
				requestCaptureFunc:  nil,
			},
		}, {
			name: "post method",
			args: args{
				method:              http.MethodPost,
				path:                "/",
				responseStatus:      http.StatusOK,
				responseObject:      nil,
				responseContentType: "JSON",
				requestCaptureFunc:  nil,
			},
		}, {
			name: "response",
			args: args{
				method:              http.MethodGet,
				path:                "/",
				responseStatus:      http.StatusOK,
				responseObject:      map[string]string{"foo": "bar"},
				responseContentType: "JSON",
				requestCaptureFunc:  nil,
			},
		}, {
			name: "response with time out",
			args: args{
				method:              http.MethodGet,
				path:                "/",
				responseStatus:      http.StatusOK,
				delay:               500 * time.Millisecond,
				responseObject:      map[string]string{"foo": "bar"},
				responseContentType: "JSON",
				requestCaptureFunc:  nil,
			},
		}, {
			name: "capture func",
			args: args{
				method:              http.MethodGet,
				path:                "/",
				responseStatus:      http.StatusOK,
				responseObject:      map[string]string{"foo": "bar"},
				responseContentType: "JSON",
				requestCaptureFunc: func(body []byte, headers http.Header) {
					//go default headers
					expectedHeaders := http.Header{
						"Accept-Encoding": []string{"gzip"},
						"User-Agent":      []string{"Go-http-client/1.1"},
					}
					assert.Equal(t, expectedHeaders, headers)
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := StartDefaultHttpServer()

			s.AddInteraction(tt.args.method, tt.args.path, tt.args.responseStatus, tt.args.responseObject, tt.args.responseContentType, tt.args.requestCaptureFunc, option.WithResponseDelay(tt.args.delay))
			uri := fmt.Sprintf("http://localhost:%d", s.Port)

			client := &http.Client{}
			client.Timeout = 300 * time.Millisecond

			req, _ := http.NewRequest(tt.args.method, uri, nil)
			resp, errReq := client.Do(req)

			if tt.args.delay < client.Timeout {
				assert.NoError(t, errReq)
			} else {
				if err, ok := errReq.(net.Error); ok {
					assert.True(t, err.Timeout())
				}
				return
			}
			assert.Equal(t, tt.args.responseStatus, resp.StatusCode)

			if tt.args.responseObject != nil {
				actualBody, _ := ioutil.ReadAll(resp.Body)
				_ = resp.Body.Close()

				expectedBody, _ := jsoniter.Marshal(tt.args.responseObject)
				assert.Equal(t, expectedBody, actualBody)
			}
		})
	}
}

func TestMockServer_AddInteractionConcurrently(t *testing.T) {
	server := StartDefaultHttpServer()
	response := map[string]string{"foo": "bar"}
	responseContentType := "JSON"
	uri := fmt.Sprintf("http://localhost:%d/entitlement", server.Port)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		go func(index int) {
			wg.Add(1)
			defer wg.Done()

			server.AddInteraction(http.MethodPost, "/entitlement", http.StatusAccepted, response, responseContentType, nil)
			req, _ := http.NewRequest(http.MethodPost, uri, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to call mock: %v", err)
			}

			assert.Equalf(t, http.StatusAccepted, resp.StatusCode, "index: %v", index)
		}(i)
	}

	wg.Wait()
}

func TestMockServer_CaptureFunc(t *testing.T) {
	times := 3
	counter := 0
	counterFunc := func(body []byte, headers http.Header) {
		counter++
	}

	s := StartDefaultHttpServer()
	uri := fmt.Sprintf("http://localhost:%d", s.Port)

	for i := 0; i < times; i++ {
		s.AddInteraction(http.MethodGet, "/", http.StatusOK, nil, "JSON", counterFunc)
		resp, _ := http.Get(uri)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	assert.Equal(t, times, counter)
}
