package whoami

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/client"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/errdefs"
)

func newClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := client.New(client.Options{APIKey: "test-key", APIURL: srv.URL})
	require.NoError(t, err)
	return c
}

func TestGet(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/whoami", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("X-API-Key"))

		_, _ = w.Write([]byte(`{
			"company": {"id": 65906048, "name": "Acme", "email": "a@example.com"},
			"project": {"id": 63994348, "name": "Proj"},
			"api_key": {
				"key_id": "key-1",
				"name": "my key",
				"expire_time": null,
				"create_time": "2025-11-07T11:58:15+08:00"
			}
		}`))
	})

	who, err := Get(context.Background(), c)
	require.NoError(t, err)

	assert.Equal(t, int64(65906048), who.Company.ID)
	assert.Equal(t, "Acme", who.Company.Name)
	assert.Equal(t, "a@example.com", who.Company.Email)
	assert.Equal(t, "Proj", who.Project.Name)
	require.NotNil(t, who.APIKey)
	assert.Equal(t, "key-1", who.APIKey.KeyID)
	assert.Nil(t, who.APIKey.ExpireTime)
	assert.True(t, who.APIKey.CreateTime.Equal(
		time.Date(2025, 11, 7, 3, 58, 15, 0, time.UTC)))
}

func TestGetReportsAnHTTPFailure(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "invalid api key"}`))
	})

	_, err := Get(context.Background(), c)
	require.Error(t, err)

	var authErr *errdefs.AuthenticationError
	assert.True(t, errors.As(err, &authErr))
	assert.Contains(t, err.Error(), "invalid api key")
}

func TestGetReportsAnUnparseableBody(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})

	_, err := Get(context.Background(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse whoami response")
}

func TestFields(t *testing.T) {
	tests := []struct {
		name string
		resp Response
		want []Field
	}{
		{
			name: "names",
			resp: Response{
				Company: Company{ID: 1, Name: "Acme"},
				Project: Project{ID: 2, Name: "Proj"},
				APIKey:  &APIKey{KeyID: "key-1", Name: "my key"},
			},
			want: []Field{
				{Label: "Company", Value: "Acme"},
				{Label: "Project", Value: "Proj"},
				{Label: "API key", Value: "my key"},
			},
		},
		{
			name: "IDs stand in for missing names",
			resp: Response{
				Company: Company{ID: 1},
				Project: Project{ID: 2, Name: "  "},
				APIKey:  &APIKey{KeyID: "key-1"},
			},
			want: []Field{
				{Label: "Company", Value: "1"},
				{Label: "Project", Value: "2"},
				{Label: "API key", Value: "key-1"},
			},
		},
		{
			name: "parts with neither are left out",
			resp: Response{
				Project: Project{Name: "Proj"},
				APIKey:  &APIKey{},
			},
			want: []Field{{Label: "Project", Value: "Proj"}},
		},
		{
			name: "no API key",
			resp: Response{Company: Company{Name: "Acme"}},
			want: []Field{{Label: "Company", Value: "Acme"}},
		},
		{
			name: "nothing",
			resp: Response{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.resp.Fields())
		})
	}
}
