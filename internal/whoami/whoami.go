// Package whoami asks the control plane who an API key belongs to. The SDK
// does not wrap the endpoint yet, so the request is made with the SDK's own
// HTTP client and credentials.
package whoami

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/client"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/transport"
)

const path = "/whoami"

// Company is the company the API key belongs to.
type Company struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Project is the project the API key belongs to.
type Project struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// APIKey describes the API key the request authenticated with.
type APIKey struct {
	KeyID string `json:"key_id"`
	Name  string `json:"name"`
	// ExpireTime is nil when the key never expires.
	ExpireTime *time.Time `json:"expire_time"`
	CreateTime time.Time  `json:"create_time"`
}

// Response is what the whoami endpoint returns.
type Response struct {
	Company Company `json:"company"`
	Project Project `json:"project"`
	// APIKey is only set when the request authenticated with an API key.
	APIKey *APIKey `json:"api_key,omitempty"`
}

// Get calls the whoami endpoint with the client's credentials.
func Get(ctx context.Context, c *client.Client) (*Response, error) {
	t := c.Transport()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.APIURL()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build whoami request: %w", err)
	}
	for k, v := range t.Headers() {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-API-Key", t.APIKey())
	req.Header.Set("Accept", "application/json")

	resp, err := t.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("call whoami: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read whoami response: %w", err)
	}
	if err := transport.Check(resp, body); err != nil {
		return nil, err
	}

	var who Response
	if err := json.Unmarshal(body, &who); err != nil {
		return nil, fmt.Errorf("parse whoami response: %w", err)
	}

	return &who, nil
}

// Field is one named part of an identity, such as its company.
type Field struct {
	Label string
	Value string
}

// Fields lists the company, project and API key, each named by its name,
// falling back to its ID, and left out when it has neither.
func (r *Response) Fields() []Field {
	var fields []Field
	add := func(label, value string) {
		if value != "" {
			fields = append(fields, Field{Label: label, Value: value})
		}
	}

	add("Company", describe(r.Company.Name, idString(r.Company.ID)))
	add("Project", describe(r.Project.Name, idString(r.Project.ID)))
	if r.APIKey != nil {
		add("API key", describe(r.APIKey.Name, r.APIKey.KeyID))
	}

	return fields
}

// describe picks the name when there is one, and the ID otherwise.
func describe(name, id string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return strings.TrimSpace(id)
}

// idString renders a numeric ID, treating zero as absent.
func idString(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
