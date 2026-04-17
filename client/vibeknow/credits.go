package vibeknow

import (
	"context"
	"fmt"
)

type Balance struct {
	TotalBalance int64 `json:"total_balance"`
	TotalFrozen  int64 `json:"total_frozen"`
}

func (c *Client) GetBalance(ctx context.Context) (*Balance, error) {
	var b Balance
	if err := c.http.Do(ctx, "GET", "/v1/credits/balance", nil, &b); err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	return &b, nil
}
