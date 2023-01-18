package http_client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	handler = http.NotFound
	hs      *httptest.Server
	client  *HttpClient
)

const wait1sUri = "/wait1s"

var someError = errors.New("some error")

func TestMain(t *testing.M) {
	var exitCode int
	{
		hs = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if req.RequestURI == wait1sUri {
				time.Sleep(time.Second)
			}
			handler(rw, req)
		}))
		defer hs.Close()

		client = NewHttpClient(HttpClientParams{
			BaseUrl: hs.URL,
			ErrorHandler: func(reader io.Reader) error {
				return someError
			},
			Headers:            nil,
			Timeout:            time.Second,
			DebugMode:          true,
			ContextRequestId:   "0",
			HeaderKeyRequestID: "X-Request-Id",
		})

		exitCode = t.Run()
	}

	time.Sleep(time.Second)
	os.Exit(exitCode)
}

func TestClient(t *testing.T) {
	headers := map[string]string{
		"Header1": "Value-of-header1",
		"Header2": "Value-of-header2",
	}
	type payloadStruct struct {
		Data string `json:"data"`
	}
	type responseStruct struct {
		Data string `json:"data"`
	}

	payloadData := payloadStruct{Data: "text1"}
	responseData := responseStruct{Data: "text2"}

	// здесь будем делать обычный запрос и убеждаться что он доходит со всеми заголовками и приходит норм ответ
	handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/qwe" {
			t.Error("Bad path!")
		}

		// проверяем что присутствуют все заголовки
		for i, v := range headers {
			assert.Equal(t, req.Header.Get(i), v)
		}

		var reqBody payloadStruct
		err := json.NewDecoder(req.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, payloadData, reqBody)
		respData, err := json.Marshal(responseData)
		assert.NoError(t, err)
		io.WriteString(rw, string(respData))
	}
	var resp responseStruct
	_, err := client.DoRequestWithOptions(RequestOptions{
		Ctx:     context.Background(),
		Method:  "POST",
		Url:     "/qwe",
		Headers: headers,
		Payload: payloadData,
		Result:  &resp,
		AfterCallback: func(req *http.Request, resp *http.Response) {
			b := bytes.NewBuffer(make([]byte, 0))
			reader := io.TeeReader(resp.Body, b)

			buf := new(strings.Builder)
			_, err := io.Copy(buf, reader)
			assert.NoError(t, err)
			assert.Equal(t, "{\"data\":\"text2\"}", buf.String())

			resp.Body = ioutil.NopCloser(b)

			rBuf := &strings.Builder{}
			_, err = io.Copy(rBuf, req.Body)
			assert.NoError(t, err)
			assert.Equal(t, "{\"data\":\"text1\"}", rBuf.String())
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, resp, responseData)
}

func TestGobClient(t *testing.T) {
	type payloadStruct struct {
		Data string
	}
	type responseStruct struct {
		Data2 string
	}

	payloadData := payloadStruct{Data: "text1"}
	responseData := responseStruct{Data2: "text2"}

	// здесь будем делать обычный запрос и убеждаться что он доходит со всеми заголовками и приходит норм ответ
	handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/qwe2" {
			t.Error("Bad path!")
		}

		var reqBody payloadStruct
		err := GobDecoder(req.Body, &reqBody)
		assert.NoError(t, err)
		assert.Equal(t, payloadData, reqBody)
		respData, err := GobEncoder(responseData)
		assert.NoError(t, err)
		// todo: здесь надо писать из reader-а во writer
		p := make([]byte, 100)
		for {
			n, err := respData.Read(p)
			if err != nil && err != io.EOF {
				assert.NoError(t, err)
			}
			rw.Write(p[:n])
			if err == io.EOF {
				break
			}
		}
	}
	var resp responseStruct
	_, err := client.DoRequestWithOptions(RequestOptions{
		Ctx:                   context.Background(),
		Method:                "POST",
		Url:                   "/qwe2",
		Payload:               payloadData,
		Result:                &resp,
		RequestPayloadEncoder: GobEncoder,
		RequestPayloadDecoder: GobDecoder,
	})
	assert.NoError(t, err)
	assert.Equal(t, resp, responseData)
}

func TestContextCancel(t *testing.T) {
	handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/qwe" {
			t.Error("Bad path!")
		}
		select {
		case <-req.Context().Done():
			return
		case <-time.After(time.Second * 2):
			t.Error("context didn't cancel")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond * 100)
		cancel()
	}()
	_, err := client.PostRequest(ctx, "/qwe", nil, nil, nil)
	assert.Error(t, err)
}

func TestTimeout(t *testing.T) {
	handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/qwe" {
			t.Error("Bad path!")
		}
		select {
		case <-req.Context().Done():
			return
		case <-time.After(time.Second * 3):
			t.Error("timeout didn't work")
		}
	}

	_, err := client.PostRequest(context.Background(), "/qwe", nil, nil, nil)
	assert.Error(t, err)
}

func TestErrorHandler(t *testing.T) {
	handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/qwe" {
			t.Error("Bad path!")
		}
		// здесь надо менять статус
		rw.WriteHeader(400)
		err := NewErr(2, "222", nil)
		io.WriteString(rw, err.String())
	}
	// здесь будем проверять на ошибку
	_, err := client.PostRequest(context.Background(), "/qwe", nil, nil, nil)
	assert.Error(t, err)
	if !errors.Is(err, someError) {
		t.Error("incorrect error")
	}
}

func TestError404(t *testing.T) {
	_, err := client.DoRequestWithOptions(RequestOptions{
		Ctx:    context.Background(),
		Method: "GET",
		Url:    "/herrnotfound404",
	})
	t.Log(err)
	assert.Error(t, err)
}

func TestCtxCancel(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond)
	_, err := client.DoRequestWithOptions(RequestOptions{
		Ctx:    ctx,
		Method: "GET",
		Url:    wait1sUri,
	})
	t.Log(err)
	assert.Error(t, err)
}
