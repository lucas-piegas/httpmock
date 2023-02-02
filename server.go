package httpmock

import (
	"context"
	"fmt"
	"github.com/httpmock/option"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type timedOut bool

type Server struct {
	Interactions *Interactions
	Port         int
	errorChannel chan error
	httpServer   *http.Server
	config       *Config
	logger       *zap.Logger
}

type Config struct {
	StartupWaitTimeout  time.Duration
	ShutdownWaitTimeout time.Duration
}

var defaultConfig = &Config{
	StartupWaitTimeout:  3 * time.Second,
	ShutdownWaitTimeout: 15 * time.Second,
}

func StartDefaultHttpServer() *Server {
	return NewServer().
		WithConfig(defaultConfig).
		WithLogger(zap.L().With(zap.String("mock", "HTTP_MOCK_SERVER"))).
		Start()
}

func NewServer() *Server {
	return &Server{
		Interactions: NewInteractions(nil),
		errorChannel: make(chan error),
	}
}

func (s *Server) WithLogger(logger *zap.Logger) *Server {
	s.logger = logger
	s.Interactions = NewInteractions(s.logger)
	return s
}

func (s *Server) WithConfig(config *Config) *Server {
	s.config = config
	return s
}

func (s *Server) Start() *Server {
	router := gin.Default()
	s.Port = findFreePort(s.logger)
	s.httpServer = &http.Server{Addr: fmt.Sprintf(":%d", s.Port), Handler: router}
	router.NoRoute(s.handler)

	go func() {
		s.logger.Info("Starting mock web server", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil {
			s.errorChannel <- err
		}
	}()

	if timeout, er := wait(s.config.StartupWaitTimeout, s.errorChannel); timeout == false {
		s.logger.Panic("failed to start http mock server, reason - timeout", zap.Error(er))
	} else {
		s.logger.Info("Started mock web Server", zap.String("addr", s.httpServer.Addr))
	}

	return s
}

type errorResponse struct {
	Message string `json:"message"`
	Path    string `json:"path"`
	Method  string `json:"method"`
}

func newErr(c *gin.Context) errorResponse {
	return errorResponse{
		Message: "[MOCK WEB SERVER ERROR] does not have (any more) mock interactions for path/method",
		Path:    c.Request.URL.Path,
		Method:  c.Request.Method,
	}
}

func (s *Server) handler(c *gin.Context) {
	bodyBytes := s.getBody(c)

	s.logger.Info("request to mock server", zap.String("method", c.Request.Method), zap.Any("url", c.Request.URL), zap.Any("headers", c.Request.Header), zap.String("body", string(bodyBytes)))

	mock := s.Interactions.NextInteraction(c.Request.Method, c.Request.URL.Path)
	if mock != nil {
		if mock.DelayResponse > 0 {
			s.logger.Info("delaying response", zap.Duration("duration", mock.DelayResponse))
			time.Sleep(mock.DelayResponse)
		}
		mock.Capture(bodyBytes, c.Request.Header)
		if mock.ResponseObject != nil {
			resp, _ := jsoniter.Marshal(mock.ResponseObject)
			s.logger.Info("responding with", zap.Int("httpStatus", mock.ResponseHttpStatus), zap.String("body", string(resp)))

			if mock.ResponseContentType == "XML" {
				c.XML(mock.ResponseHttpStatus, mock.ResponseObject)
				return
			}
			c.JSON(mock.ResponseHttpStatus, mock.ResponseObject)
		} else {
			s.logger.Info("responding with status code only", zap.Int("httpStatus", mock.ResponseHttpStatus))
			c.Status(mock.ResponseHttpStatus)
		}
	} else {
		s.logger.Warn("responding with error 501 since no interactions were found")
		c.JSON(http.StatusNotImplemented, newErr(c))
	}
}

func (s *Server) getBody(c *gin.Context) []byte {
	defer func() {
		_ = c.Request.Body.Close()
	}()
	bodyBytes, _ := ioutil.ReadAll(c.Request.Body)
	return bodyBytes
}

// AddInteraction adds a new interaction into the server
func (s *Server) AddInteraction(method string, path string, responseStatus int, responseObject interface{}, responseContentType string, requestCaptureFunc RequestCaptureFunc, opts ...option.HttpMockOptionFunc) {
	s.Interactions.Add(method, path, responseStatus, responseObject, responseContentType, requestCaptureFunc, opts...)
}

func (s *Server) Reset() {
	s.Interactions.Reset()
}

func (s *Server) Shutdown() {
	s.logger.Info("Shutting down mock web server HTTP Server", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.Shutdown(context.Background()); err != nil {
		s.logger.Error("Failed to shut down server", zap.Error(err))
	}
	if timeout, err := wait(s.config.ShutdownWaitTimeout, s.errorChannel); timeout {
		s.logger.Error("timed out waiting for mock web Server to shut down")
	} else {
		s.logger.Sugar().Infof("Server shut down: %v", err)
	}
}

func wait(timeout time.Duration, errorChannel chan error) (timedOut, error) {
	select {
	case err := <-errorChannel:
		return false, err
	case <-time.After(timeout):
		return true, nil
	}
}

func findFreePort(logger *zap.Logger) (port int) {
	addr, resolveAddressError := net.ResolveTCPAddr("tcp", "localhost:0")
	if resolveAddressError != nil {
		logger.Sugar().Panicf("unable to resolve a random IP address on localhost : %v", resolveAddressError)
	}
	listen, listenError := net.ListenTCP("tcp", addr)
	if listenError != nil {
		logger.Sugar().Panicf("unable to listen on %v which assigning random port : %v", addr, listenError)
	}
	if listenCloseError := listen.Close(); listenCloseError != nil {
		logger.Sugar().Panicf("unable to Close TCP listener on %v : %v", addr, listenCloseError)
	}

	port = listen.Addr().(*net.TCPAddr).Port
	return
}
