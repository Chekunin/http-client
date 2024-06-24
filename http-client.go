package http_client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	wrapErr "github.com/Chekunin/wraperr"
	"moul.io/http2curl"
)

type DataEncoder func(payload interface{}) (io.Reader, error)
type DataDecoder func(reader io.Reader, res interface{}) error

type HttpClient struct {
	baseUrl               string
	errorHandler          func(closer io.Reader) error
	isError               func(resp *http.Response) bool
	headers               map[string]string
	httpClient2           *http.Client
	requestPayloadEncoder DataEncoder
	requestPayloadDecoder DataDecoder
	contextRequestId      string
	headerKeyRequestID    string
	debugMode             bool
}

type HttpClientParams struct {
	BaseUrl               string
	ErrorHandler          func(closer io.Reader) error
	IsError               func(resp *http.Response) bool
	Headers               map[string]string
	Timeout               time.Duration
	RequestPayloadEncoder DataEncoder
	RequestPayloadDecoder DataDecoder
	MaxIdleConnsPerHost   int
	ContextRequestId      string
	HeaderKeyRequestID    string
	DebugMode             bool
}

func NewHttpClient(params HttpClientParams) *HttpClient {
	transport := getDefaultHttpTransport()
	transport.MaxIdleConnsPerHost = params.MaxIdleConnsPerHost
	if params.MaxIdleConnsPerHost == 0 {
		transport.MaxIdleConnsPerHost = 100
	}

	client := HttpClient{
		baseUrl:      params.BaseUrl,
		errorHandler: params.ErrorHandler,
		isError:      params.IsError,
		headers:      params.Headers,
		httpClient2: &http.Client{
			Timeout:   params.Timeout,
			Transport: transport,
		},
		requestPayloadEncoder: params.RequestPayloadEncoder,
		requestPayloadDecoder: params.RequestPayloadDecoder,
		contextRequestId:      params.ContextRequestId,
		headerKeyRequestID:    params.HeaderKeyRequestID,
		debugMode:             params.DebugMode,
	}
	if client.requestPayloadEncoder == nil {
		client.requestPayloadEncoder = JsonEncoder
	}
	if client.requestPayloadDecoder == nil {
		client.requestPayloadDecoder = JsonDecoder
	}
	return &client
}

func getDefaultHttpTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func (c *HttpClient) SetHeaders(headers map[string]string) {
	c.headers = headers
}

func (c *HttpClient) PostRequest(ctx context.Context,
	url string,
	headers map[string]string,
	payload interface{},
	result interface{}) (*http.Response, error) {
	return c.DoRequest(ctx, "POST", url, headers, payload, result)
}

func (c *HttpClient) GetRequest(ctx context.Context,
	url string,
	headers map[string]string,
	result interface{}) (*http.Response, error) {
	return c.DoRequest(ctx, "GET", url, headers, nil, result)
}

type RequestOptions struct {
	Ctx                   context.Context
	Method                string
	Url                   string
	Headers               map[string]string
	Payload               interface{}
	Result                interface{}
	RequestPayloadEncoder DataEncoder
	RequestPayloadDecoder DataDecoder
	UrlParams             map[string]string
	AfterCallback         func(req *http.Request, resp *http.Response)
}

func (c HttpClient) setDefaultOptions(opt *RequestOptions) {
	if opt == nil {
		return
	}

	if opt.Ctx == nil {
		opt.Ctx = context.Background()
	}
	if opt.Method == "" {
		opt.Method = "GET"
	}
	if opt.RequestPayloadEncoder == nil {
		opt.RequestPayloadEncoder = c.requestPayloadEncoder
	}
	if opt.RequestPayloadDecoder == nil {
		opt.RequestPayloadDecoder = c.requestPayloadDecoder
	}
}

func (c *HttpClient) DoRequestWithOptions(options RequestOptions) (*http.Response, error) {
	c.setDefaultOptions(&options)
	payloadReader, err := options.RequestPayloadEncoder(options.Payload)
	if err != nil {
		err = wrapErr.Wrap(fmt.Errorf("requestPayloadEncoder"), err)
		return nil, err
	}
	reqBuffer := bytes.NewBuffer(make([]byte, 0))
	if options.AfterCallback != nil {
		b := bytes.NewBuffer(make([]byte, 0))
		reader := io.TeeReader(payloadReader, b)

		_, err = io.Copy(reqBuffer, reader)
		if err != nil {
			return nil, err
		}
		payloadReader = ioutil.NopCloser(b)
	}
	req, err := http.NewRequestWithContext(
		options.Ctx,
		options.Method,
		fmt.Sprintf("%s%s", c.baseUrl, options.Url),
		payloadReader,
	)
	if err != nil {
		err = wrapErr.Wrap(fmt.Errorf("new request with context"), err)
		return nil, err
	}

	q := req.URL.Query()
	for key, val := range options.UrlParams {
		q.Add(key, val)
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Accept", "application/json; charset=utf-8")
	if options.Ctx != nil {
		if requestID, has := c.fromContextRequestId(options.Ctx); has {
			req.Header.Set(c.headerKeyRequestID, requestID)
		}
	}
	for i, v := range c.headers {
		req.Header.Set(i, v)
	}
	for i, v := range options.Headers {
		req.Header.Set(i, v)
	}

	var curl *http2curl.CurlCommand
	var t time.Time
	if c.debugMode {
		curl, _ = http2curl.GetCurlCommand(req)
		t = time.Now()
	}
	resp, err := c.httpClient2.Do(req)
	requestTook := fmt.Sprintf("request took %f microseconds", float64(time.Now().UnixNano()-t.UnixNano())/float64(time.Microsecond))
	if err != nil {
		if c.debugMode {
			err = wrapErr.Wrap(fmt.Errorf("curl: %s", curl), err)
			err = wrapErr.Wrap(fmt.Errorf("%s", requestTook), err)
		}
		err = wrapErr.Wrap(fmt.Errorf("do http request"), err)
		return nil, err
	}
	defer resp.Body.Close()

	if options.AfterCallback != nil {
		req.Body = ioutil.NopCloser(reqBuffer)
		options.AfterCallback(req, resp)
	}

	if c.defaultIsError(resp) {
		var err error
		if c.debugMode {
			err = wrapErr.Wrap(fmt.Errorf("curl: %s", curl), err)
			err = wrapErr.Wrap(fmt.Errorf("%s", requestTook), err)
		}
		err = wrapErr.Wrap(fmt.Errorf("http status code=%d curl", resp.StatusCode), c.defaultErrorHandler(resp.Body))
		return nil, err
	}
	if options.Result != nil {
		if err := options.RequestPayloadDecoder(resp.Body, options.Result); err != nil {
			if c.debugMode {
				err = wrapErr.Wrap(fmt.Errorf("curl: %s", curl), err)
				err = wrapErr.Wrap(fmt.Errorf("%s", requestTook), err)
			}
			err = wrapErr.Wrap(fmt.Errorf("decode resp.Body"), err)
			return nil, err
		}
	}

	return resp, nil
}

func (c *HttpClient) DoRequest(ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	payload interface{},
	result interface{}) (*http.Response, error) {
	return c.DoRequestWithOptions(RequestOptions{
		Ctx:     ctx,
		Method:  method,
		Url:     url,
		Headers: headers,
		Payload: payload,
		Result:  result,
	})
}

func (c *HttpClient) defaultErrorHandler(reader io.Reader) error {
	if c.errorHandler != nil {
		return c.errorHandler(reader)
	}
	var e Err
	if err := c.requestPayloadDecoder(reader, &e); err != nil {
		panic(err)
	}
	return e
}

func (c *HttpClient) defaultIsError(resp *http.Response) bool {
	return resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest
}

func (c *HttpClient) fromContextRequestId(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(c.contextRequestId).(string)
	return u, ok
}
