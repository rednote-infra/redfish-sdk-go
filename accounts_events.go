package redfish

import (
	"context"
	"errors"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func (c *Client) AddAccount(ctx context.Context, account models.AccountCreate) (*models.Account, error) {
	if strings.TrimSpace(account.UserName) == "" || strings.TrimSpace(account.RoleID) == "" {
		return nil, errors.New("account requires UserName and RoleId")
	}
	service, err := c.GetAccountService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(service.Accounts, "Accounts")
	if err != nil {
		return nil, err
	}
	r, err := c.Post(ctx, uri, account)
	if err != nil {
		return nil, err
	}
	var result models.Account
	if len(r.Body) > 0 {
		if err = r.Decode(&result); err != nil {
			return nil, err
		}
	}
	if result.ODataID == nil && r.Header.Get(HeaderLocation) != "" {
		result.ODataID = Ptr(r.Header.Get(HeaderLocation))
	}
	return &result, nil
}
func (c *Client) findAccount(ctx context.Context, username string) (*models.Account, error) {
	accounts, err := c.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var match *models.Account
	for _, account := range accounts {
		if Value(account.UserName) == username {
			if match != nil {
				return nil, errors.New("account username is ambiguous")
			}
			copy := account
			match = &copy
		}
	}
	if match == nil {
		return nil, ErrNotFound
	}
	return match, nil
}
func (c *Client) UpdateAccount(ctx context.Context, username string, account models.AccountPatch) (*Response, error) {
	current, err := c.findAccount(ctx, username)
	if err != nil {
		return nil, err
	}
	return c.patchWithETag(ctx, Value(current.ODataID), account)
}
func (c *Client) DeleteAccount(ctx context.Context, username string) error {
	current, err := c.findAccount(ctx, username)
	if err != nil {
		return err
	}
	return c.Delete(ctx, Value(current.ODataID))
}

// Subscribe accepts the typed subscription contract, preserving extension fields in Extra.
func (c *Client) Subscribe(ctx context.Context, subscription models.Subscription) (*models.Subscription, error) {
	if strings.TrimSpace(Value(subscription.Destination)) == "" {
		return nil, errors.New("subscription destination must not be empty")
	}
	if subscription.Protocol == nil {
		subscription.Protocol = Ptr("Redfish")
	}
	service, err := c.GetEventService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(service.Subscriptions, "Subscriptions")
	if err != nil {
		return nil, err
	}
	r, err := c.Post(ctx, uri, subscription)
	if err != nil {
		return nil, err
	}
	var result models.Subscription
	if err = r.Decode(&result); err != nil {
		return nil, err
	}
	if result.ODataID == nil && r.Header.Get(HeaderLocation) != "" {
		result.ODataID = Ptr(r.Header.Get(HeaderLocation))
	}
	return &result, nil
}
func (c *Client) DeleteSubscription(ctx context.Context, id string) error {
	subscription, err := c.GetSubscription(ctx, id)
	if err != nil {
		return err
	}
	return c.Delete(ctx, Value(subscription.ODataID))
}
func (c *Client) SubmitTestEvent(ctx context.Context, event models.EventRecord) (*models.RedfishResponse, error) {
	service, err := c.GetEventService(ctx)
	if err != nil {
		return nil, err
	}
	actions, err := c.resourceActions(ctx, Value(service.ODataID))
	if err != nil {
		return nil, err
	}
	action, err := findAction(actions, "SubmitTestEvent")
	if err != nil {
		return nil, err
	}
	return c.postOperation(ctx, action.Target, event)
}

type PolicyCreate struct {
	ID               string                  `json:"Id"`
	Name             string                  `json:"Name"`
	TriggerCondition models.PolicyCondition  `json:"TriggerCondition"`
	Response         []models.PolicyResponse `json:"Response"`
	Enabled          *bool                   `json:"Enabled,omitempty"`
	OEM              *Object                 `json:"Oem,omitempty"`
}
type PolicyUpdate struct {
	TriggerCondition *models.PolicyCondition  `json:"TriggerCondition,omitempty"`
	Response         *[]models.PolicyResponse `json:"Response,omitempty"`
	Enabled          *bool                    `json:"Enabled,omitempty"`
	OEM              *Object                  `json:"Oem,omitempty"`
}

func (c *Client) CreatePolicy(ctx context.Context, request PolicyCreate) (*models.Policy, error) {
	if strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Name) == "" || len(request.Response) == 0 {
		return nil, errors.New("policy requires Id, Name and at least one Response")
	}
	service, err := c.GetPolicyService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(service.Policies, "Policies")
	if err != nil {
		return nil, err
	}
	r, err := c.Post(ctx, uri, request)
	if err != nil {
		return nil, err
	}
	var result models.Policy
	if err = r.Decode(&result); err != nil {
		return nil, err
	}
	if result.ODataID == nil && r.Header.Get(HeaderLocation) != "" {
		result.ODataID = Ptr(r.Header.Get(HeaderLocation))
	}
	return &result, nil
}
func (c *Client) UpdatePolicy(ctx context.Context, id string, request PolicyUpdate) (*Response, error) {
	policy, err := c.GetPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.patchWithETag(ctx, Value(policy.ODataID), request)
}
func (c *Client) EnablePolicy(ctx context.Context, id string) (*Response, error) {
	return c.UpdatePolicy(ctx, id, PolicyUpdate{Enabled: Ptr(true)})
}
func (c *Client) DisablePolicy(ctx context.Context, id string) (*Response, error) {
	return c.UpdatePolicy(ctx, id, PolicyUpdate{Enabled: Ptr(false)})
}
func (c *Client) DeletePolicy(ctx context.Context, id string) error {
	policy, err := c.GetPolicy(ctx, id)
	if err != nil {
		return err
	}
	return c.Delete(ctx, Value(policy.ODataID))
}
