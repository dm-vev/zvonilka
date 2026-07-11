package pgstore

import (
	"reflect"
	"testing"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func TestPrivacyRulesPersistenceMapping(t *testing.T) {
	t.Parallel()

	want := domainuser.Privacy{
		ShowStatus:                            domainuser.PrivacyRule{Base: domainuser.VisibilityContacts, AllowUserIDs: []string{"user"}},
		ShowProfilePhoto:                      domainuser.PrivacyRule{Base: domainuser.VisibilityNobody, AllowChatIDs: []string{"chat"}},
		ShowProfileAudio:                      domainuser.PrivacyRule{Base: domainuser.VisibilityEveryone, RestrictBots: true},
		ShowBirthdate:                         domainuser.PrivacyRule{Base: domainuser.VisibilityNobody},
		ShowBio:                               domainuser.PrivacyRule{Base: domainuser.VisibilityContacts},
		ShowPhoneNumber:                       domainuser.PrivacyRule{Base: domainuser.VisibilityNobody},
		AllowFindingByPhoneNumber:             domainuser.PrivacyRule{Base: domainuser.VisibilityEveryone},
		ShowLinkInForwardedMessages:           domainuser.PrivacyRule{Base: domainuser.VisibilityEveryone},
		AllowChatInvites:                      domainuser.PrivacyRule{Base: domainuser.VisibilityContacts},
		AllowPrivateVoiceAndVideoNoteMessages: domainuser.PrivacyRule{Base: domainuser.VisibilityNobody},
		AllowCalls:                            domainuser.PrivacyRule{Base: domainuser.VisibilityContacts, AllowPremiumUsers: true},
		AllowPeerToPeerCalls:                  domainuser.PrivacyRule{Base: domainuser.VisibilityNobody},
		AutosaveGifts:                         domainuser.PrivacyRule{Base: domainuser.VisibilityEveryone, RestrictUserIDs: []string{"blocked"}},
	}
	var got domainuser.Privacy
	applyPrivacyRules(&got, privacyRulesFromPrivacy(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("privacy rules round trip = %+v, want %+v", got, want)
	}
}
