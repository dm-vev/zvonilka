package user_test

import (
	"testing"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func TestPrivacyRuleAllowsExceptionsAndFlags(t *testing.T) {
	t.Parallel()

	rule := domainuser.PrivacyRule{
		Base:              domainuser.VisibilityContacts,
		AllowUserIDs:      []string{"allowed"},
		RestrictUserIDs:   []string{"blocked"},
		AllowChatIDs:      []string{"allowed-chat"},
		RestrictChatIDs:   []string{"blocked-chat"},
		AllowPremiumUsers: true,
		AllowBots:         true,
	}
	tests := []struct {
		name   string
		viewer domainuser.PrivacyViewer
		want   bool
	}{
		{"contact", domainuser.PrivacyViewer{IsContact: true}, true},
		{"stranger", domainuser.PrivacyViewer{}, false},
		{"allowed user", domainuser.PrivacyViewer{UserID: "allowed"}, true},
		{"restricted user wins", domainuser.PrivacyViewer{UserID: "blocked", IsContact: true}, false},
		{"allowed chat", domainuser.PrivacyViewer{ChatIDs: []string{"allowed-chat"}}, true},
		{"restricted chat wins", domainuser.PrivacyViewer{ChatIDs: []string{"blocked-chat"}, IsPremium: true}, false},
		{"premium", domainuser.PrivacyViewer{IsPremium: true}, true},
		{"bot", domainuser.PrivacyViewer{IsBot: true}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rule.Allows(test.viewer); got != test.want {
				t.Fatalf("Allows() = %v, want %v", got, test.want)
			}
		})
	}
}
