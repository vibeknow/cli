package account

import "context"

type User struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
}

func (c *Client) Whoami(ctx context.Context) (*User, error) {
	var u User
	if err := c.http.Do(ctx, "GET", "/v1/user/profile", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
