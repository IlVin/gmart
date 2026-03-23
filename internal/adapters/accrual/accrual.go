package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"gmart/internal/domain"
	"gmart/internal/dto"
)

var (
	ErrNoContent       = errors.New("order not registered in accrual")
	ErrTooManyRequests = errors.New("rate limited")
	ErrInternalError   = errors.New("accrual internal server error")
)

type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
}

func NewClient(baseURL *url.URL) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{MaxIdleConnsPerHost: 100},
		},
	}
}

func (c *Client) Fetch(ctx context.Context, order domain.OrderNumber) (*dto.AccrualResponse, error) {
	u := c.baseURL.JoinPath("api", "orders", order.String()).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {

	case http.StatusOK:
		var res dto.AccrualResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}
		return &res, nil

	case http.StatusNoContent:
		return nil, ErrNoContent

	case http.StatusTooManyRequests:
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if ra <= 0 {
			ra = 60
		}
		return &dto.AccrualResponse{RetryAfter: time.Duration(ra) * time.Second}, ErrTooManyRequests

	case http.StatusInternalServerError:
		return nil, ErrInternalError

	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}
