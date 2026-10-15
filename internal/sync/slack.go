package sync

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kevholditch/go-pagerduty-slack-sync/internal/compare"
	"github.com/slack-go/slack"
)

type slackClient struct{ client *slack.Client }

func newSlackClient(token string) *slackClient {
	return &slackClient{client: slack.New(token, slack.OptionHTTPClient(&http.Client{Timeout: 20 * time.Second}))}
}
func (s *slackClient) Sync(ctx context.Context, handle, name string, emails []string) error {
	// Resolve every user before making any group changes.
	var ids []string
	seen := map[string]bool{}
	for _, email := range emails {
		u, err := s.client.GetUserByEmailContext(ctx, email)
		if err != nil {
			return fmt.Errorf("resolve Slack user: %w", err)
		}
		if u.ID == "" || u.Deleted || u.IsBot || u.IsRestricted || u.IsUltraRestricted {
			return fmt.Errorf("on-call user cannot belong to a Slack user group")
		}
		if !seen[u.ID] {
			ids = append(ids, u.ID)
			seen[u.ID] = true
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("refusing to replace membership with an empty list")
	}
	groups, err := s.client.GetUserGroupsContext(ctx, slack.GetUserGroupsOptionIncludeDisabled(true))
	if err != nil {
		return err
	}
	var group *slack.UserGroup
	for i := range groups {
		if strings.EqualFold(groups[i].Handle, handle) {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		created, err := s.client.CreateUserGroupContext(ctx, slack.UserGroup{Name: name, Handle: handle})
		if err != nil {
			return err
		}
		group = &created
	}
	if group.DateDelete != 0 {
		return fmt.Errorf("group @%s is disabled; enable it manually before syncing", handle)
	}
	if group.Name != name {
		if _, err := s.client.UpdateUserGroupContext(ctx, group.ID, slack.UpdateUserGroupsOptionName(name)); err != nil {
			return err
		}
	}
	members, err := s.client.GetUserGroupMembersContext(ctx, group.ID)
	if err != nil {
		return err
	}
	if !compare.Array(ids, members) {
		_, err = s.client.UpdateUserGroupMembersContext(ctx, group.ID, strings.Join(ids, ","))
		return err
	}
	return nil
}
