package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Chat represents a Microsoft Teams conversation that the user has pinned.
type Chat struct {
	ID       string
	Topic    string
	ChatType string // "oneOnOne", "group", "meeting"
	WebURL   string
}

// DeviceCodeInfo holds the values returned by the device code endpoint.
type DeviceCodeInfo struct {
	UserCode        string
	DeviceCode      string
	VerificationURI string
	Message         string
}

// MSGraphService is the port for Microsoft Graph operations.
// The TUI layer depends only on this interface; never on the concrete HTTP client.
type MSGraphService interface {
	// IsAuthenticated reports whether a valid cached token exists.
	IsAuthenticated() bool
	// StartDeviceCodeFlow requests a device code from Azure AD and returns the
	// values needed to display the verification URI and user code to the user.
	StartDeviceCodeFlow(ctx context.Context) (DeviceCodeInfo, error)
	// ExchangeDeviceCode polls the token endpoint using the device code and saves
	// the resulting access token. Blocks until sign-in completes or code expires.
	ExchangeDeviceCode(ctx context.Context, deviceCode string) error
	// GetPinnedChats returns the user's pinned Teams conversations.
	GetPinnedChats(ctx context.Context) ([]Chat, error)
}

// ---------------------------------------------------------------------------
// MSGraphClient implementation
// ---------------------------------------------------------------------------

const (
	msGraphScope = "Chat.Read offline_access openid profile"
	deviceCodeURL   = "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode"
	msTokenURL      = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	// List chats — viewpoint cannot be expanded or filtered server-side (Graph limitation).
	// We fetch the list then call /viewpoint per chat to check isPinned.
	chatsListURL     = "https://graph.microsoft.com/beta/me/chats?$select=id,topic,chatType,webUrl&$top=30"
	chatViewpointURL = "https://graph.microsoft.com/beta/me/chats/%s/viewpoint"
)

// tokenData is the on-disk representation of the cached OAuth token.
type tokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// MSGraphClient implements MSGraphService using raw HTTP calls (no SDK).
type MSGraphClient struct {
	httpClient *http.Client
	tokenPath  string
	token      *tokenData
	clientID   string
}

// NewMSGraphClient creates a client that caches its token at ~/.jirlab/msgraph_token.json.
// clientID must be an Azure App Registration client ID with Chat.Read delegated permission.
func NewMSGraphClient(clientID string) *MSGraphClient {
	home, _ := os.UserHomeDir()
	return &MSGraphClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		tokenPath:  filepath.Join(home, ".jirlab", "msgraph_token.json"),
		clientID:   clientID,
	}
}

// NewMSGraphClientWithPath creates a client with a custom token path (for testing).
func NewMSGraphClientWithPath(tokenPath string) *MSGraphClient {
	return &MSGraphClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		tokenPath:  tokenPath,
		clientID:   "test-client-id",
	}
}

func (c *MSGraphClient) loadToken() bool {
	if c.token != nil {
		return !c.token.ExpiresAt.Before(time.Now())
	}
	data, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return false
	}
	var t tokenData
	if err := json.Unmarshal(data, &t); err != nil {
		return false
	}
	if t.ExpiresAt.Before(time.Now()) {
		return false
	}
	c.token = &t
	return true
}

func (c *MSGraphClient) saveToken(t tokenData) error {
	if err := os.MkdirAll(filepath.Dir(c.tokenPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath, data, 0600)
}

// IsAuthenticated reports whether a valid cached token is available.
func (c *MSGraphClient) IsAuthenticated() bool {
	return c.loadToken()
}

// StartDeviceCodeFlow requests a device code from Azure AD.
func (c *MSGraphClient) StartDeviceCodeFlow(ctx context.Context) (DeviceCodeInfo, error) {
	body := url.Values{}
	body.Set("client_id", c.clientID)
	body.Set("scope", msGraphScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(body.Encode()))
	if err != nil {
		return DeviceCodeInfo{}, fmt.Errorf("build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DeviceCodeInfo{}, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return DeviceCodeInfo{}, fmt.Errorf("device code error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		UserCode        string `json:"user_code"`
		DeviceCode      string `json:"device_code"`
		VerificationURI string `json:"verification_uri"`
		Message         string `json:"message"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return DeviceCodeInfo{}, fmt.Errorf("parse device code response: %w", err)
	}

	return DeviceCodeInfo{
		UserCode:        result.UserCode,
		DeviceCode:      result.DeviceCode,
		VerificationURI: result.VerificationURI,
		Message:         result.Message,
	}, nil
}

// ExchangeDeviceCode polls the token endpoint until sign-in completes or times out.
func (c *MSGraphClient) ExchangeDeviceCode(ctx context.Context, deviceCode string) error {
	body := url.Values{}
	body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	body.Set("client_id", c.clientID)
	body.Set("device_code", deviceCode)

	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, msTokenURL, bytes.NewBufferString(body.Encode()))
		if err != nil {
			return fmt.Errorf("build token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("token request: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var result struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				return fmt.Errorf("parse token response: %w", err)
			}
			t := tokenData{
				AccessToken:  result.AccessToken,
				RefreshToken: result.RefreshToken,
				ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
			}
			if err := c.saveToken(t); err != nil {
				return fmt.Errorf("save token: %w", err)
			}
			c.token = &t
			return nil
		}

		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &errResp)
		if errResp.Error == "authorization_pending" || errResp.Error == "slow_down" {
			time.Sleep(5 * time.Second)
			continue
		}
		return fmt.Errorf("token exchange error: %s", string(raw))
	}
	return fmt.Errorf("device code flow timed out")
}

// GetPinnedChats fetches the user's pinned Teams conversations.
// The Graph API does not support bulk access to the viewpoint.isPinned property
// (neither $expand nor $filter work on it). We therefore fetch the first page of
// chats and call GET /me/chats/{id}/viewpoint per chat to check isPinned.
func (c *MSGraphClient) GetPinnedChats(ctx context.Context) ([]Chat, error) {
	if !c.loadToken() {
		return nil, fmt.Errorf("not authenticated")
	}

	// Step 1: fetch the chat list.
	type chatItem struct {
		ID       string `json:"id"`
		Topic    string `json:"topic"`
		ChatType string `json:"chatType"`
		WebURL   string `json:"webUrl"`
	}

	var items []chatItem
	nextURL := chatsListURL
	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build chats list request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("chats list request: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("chats error %d: %s", resp.StatusCode, string(raw))
		}

		var page struct {
			Value    []chatItem `json:"value"`
			NextLink string     `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("parse chats list: %w", err)
		}
		items = append(items, page.Value...)
		nextURL = page.NextLink
	}

	// Step 2: for each chat, fetch /viewpoint to check isPinned.
	// The Graph API provides no bulk access to this property.
	var chats []Chat
	for _, item := range items {
		vpURL := fmt.Sprintf(chatViewpointURL, item.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, vpURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var vp struct {
			IsPinned bool `json:"isPinned"`
		}
		if err := json.Unmarshal(raw, &vp); err != nil || !vp.IsPinned {
			continue
		}

		topic := item.Topic
		if topic == "" {
			topic = "(no topic)"
		}
		chats = append(chats, Chat{
			ID:       item.ID,
			Topic:    topic,
			ChatType: item.ChatType,
			WebURL:   item.WebURL,
		})
	}
	return chats, nil
}
