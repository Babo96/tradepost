package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type EnrollResponse struct {
	ClientID string `json:"client_id"`
	Token    string `json:"token"`
}

type IngestScan struct {
	Market        string       `json:"market"`
	ScannedAt     int64        `json:"scanned_at"`
	ClientVersion string       `json:"client_version,omitempty"`
	Items         []IngestItem `json:"items"`
}

type IngestItem struct {
	Key      string `json:"k"`
	Price    int64  `json:"p"`
	Quantity int64  `json:"q"`
	Listings int32  `json:"n"`
}

type IngestMeta struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Quality    int    `json:"quality"`
	ClassID    int    `json:"class_id"`
	SubclassID int    `json:"subclass_id"`
	Icon       int64  `json:"icon"`
}

type IngestResponse struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	Items    int `json:"items"`
	Meta     int `json:"meta"`
}

type MarketMeta struct {
	Market    string `json:"market"`
	Version   int64  `json:"version"`
	Items     int64  `json:"items"`
	LastScan  int64  `json:"last_scan"`
	UpdatedAt int64  `json:"updated_at"`
}

type SnapshotItem struct {
	Key      string `json:"k"`
	Price    int64  `json:"p"`
	Quantity int64  `json:"q"`
	Listings int32  `json:"n"`
	Scanned  int64  `json:"t"`
}

type Snapshot struct {
	Market    string         `json:"market"`
	Version   int64          `json:"version"`
	Generated int64          `json:"generated"`
	Items     []SnapshotItem `json:"items"`
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Enroll(ctx context.Context) (EnrollResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/enroll", nil)
	if err != nil {
		return EnrollResponse{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return EnrollResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return EnrollResponse{}, responseError(resp)
	}
	var result EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return EnrollResponse{}, err
	}
	return result, nil
}

func (c *Client) Ingest(ctx context.Context, scans []IngestScan, meta []IngestMeta) (IngestResponse, error) {
	payload := struct {
		V     int          `json:"v"`
		Scans []IngestScan `json:"scans"`
		Items []IngestMeta `json:"items,omitempty"`
	}{V: 1, Scans: scans, Items: meta}

	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	if err := json.NewEncoder(gz).Encode(payload); err != nil {
		return IngestResponse{}, err
	}
	if err := gz.Close(); err != nil {
		return IngestResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/ingest", bytes.NewReader(raw.Bytes()))
	if err != nil {
		return IngestResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return IngestResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return IngestResponse{}, responseError(resp)
	}
	var result IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return IngestResponse{}, err
	}
	return result, nil
}

func (c *Client) Meta(ctx context.Context) ([]MarketMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/meta", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}
	var result []MarketMeta
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Snapshot(ctx context.Context, market string) (Snapshot, error) {
	endpoint := c.BaseURL + "/v1/snapshot/" + url.PathEscape(market)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Snapshot{}, err
	}
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Snapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, responseError(resp)
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return Snapshot{}, err
		}
		defer gz.Close()
		reader = gz
	}

	var snapshot Snapshot
	if err := json.NewDecoder(reader).Decode(&snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
}
