package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type ApiRequest struct {
	Url     string
	Method  string
	Headers map[string]string
	Body    io.Reader
	Query   map[string]string
}

type ApiResponse struct {
	StatusCode int
	Body       []byte
	JsonBody   interface{}
	TextBody   string
	Headers    map[string]string
}

func NewApiRequest() *ApiRequest {
	return &ApiRequest{
		Headers: make(map[string]string),
		Query:   make(map[string]string),
	}
}

func NewApiResponse() *ApiResponse {
	return &ApiResponse{
		Headers: make(map[string]string),
	}
}

func (r *ApiRequest) SetUrl(url string) *ApiRequest {
	r.Url = url
	return r
}

func (r *ApiRequest) SetMethod(method string) *ApiRequest {
	r.Method = method
	return r
}

func (r *ApiRequest) SetHeader(key, value string) *ApiRequest {
	r.Headers[key] = value
	return r
}

func (r *ApiRequest) SetBody(body interface{}) *ApiRequest {
	// Encode the body as JSON
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			fmt.Println("Error encoding body:", err)
			return r
		}
		r.Body = bytes.NewBuffer(jsonBody)
	}
	return r
}

func (r *ApiRequest) SetQuery(key, value string) *ApiRequest {
	r.Query[key] = value
	return r
}

func (r *ApiRequest) Send() (*ApiResponse, error) {
	// Construct the full URL with query parameters
	reqUrl, err := url.Parse(r.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Add query parameters
	query := reqUrl.Query()
	for key, value := range r.Query {
		query.Set(key, value)
	}
	reqUrl.RawQuery = query.Encode()

	// Create the request
	req, err := http.NewRequest(r.Method, reqUrl.String(), r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range r.Headers {
		req.Header.Add(key, value)
	}

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Create and populate ApiResponse
	apiResp := NewApiResponse()
	apiResp.StatusCode = resp.StatusCode
	apiResp.Body = respBody

	// Copy response headers
	for key, values := range resp.Header {
		if len(values) > 0 {
			apiResp.Headers[key] = values[0]
		}
	}

	// Try to parse JSON response
	var jsonBody interface{}

	err = json.Unmarshal(respBody, &jsonBody)

	if err == nil {
		apiResp.JsonBody = jsonBody
	} else {
		apiResp.TextBody = string(respBody)
	}

	return apiResp, nil
}
