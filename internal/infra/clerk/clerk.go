package clerk

import (
	"context"
	"fmt"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
)

type Profile struct {
	ID       string
	Email    string
	Name     string
	ImageURL string
}

type Client struct{}

func New(secretKey string) *Client {
	clerk.SetKey(secretKey)
	return &Client{}
}

func (c *Client) GetProfile(ctx context.Context, userID string) (Profile, error) {
	u, err := clerkuser.Get(ctx, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("get clerk user: %w", err)
	}

	return Profile{
		ID:       u.ID,
		Email:    primaryEmail(u),
		Name:     displayName(u),
		ImageURL: stringValue(u.ImageURL),
	}, nil
}

func primaryEmail(u *clerk.User) string {
	if u.PrimaryEmailAddressID != nil {
		for _, address := range u.EmailAddresses {
			if address.ID == *u.PrimaryEmailAddressID {
				return address.EmailAddress
			}
		}
	}
	if len(u.EmailAddresses) > 0 {
		return u.EmailAddresses[0].EmailAddress
	}
	return ""
}

func displayName(u *clerk.User) string {
	parts := make([]string, 0, 2)
	if name := stringValue(u.FirstName); name != "" {
		parts = append(parts, name)
	}
	if name := stringValue(u.LastName); name != "" {
		parts = append(parts, name)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return stringValue(u.Username)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
