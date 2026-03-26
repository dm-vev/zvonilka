package gateway

import (
	"context"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// GetMyProfile returns the authenticated account profile.
func (a *api) GetMyProfile(
	ctx context.Context,
	_ *usersv1.GetMyProfileRequest,
) (*usersv1.GetMyProfileResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	userProfile, err := a.profileByID(ctx, authContext.Account.ID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.GetMyProfileResponse{Profile: userProfile}, nil
}

// SetPresence stores an explicit presence preference for the authenticated account.
func (a *api) SetPresence(
	ctx context.Context,
	req *usersv1.SetPresenceRequest,
) (*usersv1.SetPresenceResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	hideLastSeenFor := timeDuration(req.GetHideLastSeenFor())
	state, err := a.presence.SetPresence(ctx, domainpresence.SetParams{
		AccountID:       authContext.Account.ID,
		State:           presenceStateFromProto(req.GetPresence()),
		CustomStatus:    req.GetCustomStatus(),
		HideLastSeenFor: hideLastSeenFor,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	profile := profile(authContext.Account, domainpresence.Snapshot{
		AccountID:      state.AccountID,
		State:          state.State,
		CustomStatus:   state.CustomStatus,
		LastSeenHidden: false,
		LastSeenAt:     state.UpdatedAt,
		UpdatedAt:      state.UpdatedAt,
	})

	return &usersv1.SetPresenceResponse{Profile: profile}, nil
}

// GetUser returns one account profile by ID or username.
func (a *api) GetUser(
	ctx context.Context,
	req *usersv1.GetUserRequest,
) (*usersv1.GetUserResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	var profileID string
	switch lookup := req.Lookup.(type) {
	case *usersv1.GetUserRequest_UserId:
		profileID = lookup.UserId
	case *usersv1.GetUserRequest_Username:
		account, loadErr := a.identity.AccountByUsername(ctx, lookup.Username)
		if loadErr != nil {
			return nil, grpcError(loadErr)
		}
		profileID = account.ID
	default:
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	userProfile, err := a.profileByID(ctx, profileID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.GetUserResponse{Profile: userProfile}, nil
}

// SearchUsers returns indexed user hits together with lightweight profile projections.
func (a *api) SearchUsers(
	ctx context.Context,
	req *usersv1.SearchUsersRequest,
) (*usersv1.SearchUsersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetIncludeBlocked() || req.GetIncludeContactsOnly() {
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	offset, err := decodeOffset(req.GetPage(), "users")
	if err != nil {
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	result, err := a.search.Search(ctx, domainsearch.SearchParams{
		Query:  req.GetQuery(),
		Scopes: []domainsearch.SearchScope{domainsearch.SearchScopeUsers},
		Limit:  pageSize(req.GetPage()),
		Offset: offset,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	userIDs := make([]string, 0, len(result.Hits))
	seen := make(map[string]struct{}, len(result.Hits))
	for _, hit := range result.Hits {
		userID := strings.TrimSpace(hit.UserID)
		if userID == "" {
			userID = strings.TrimSpace(hit.TargetID)
		}
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}

	profiles, err := a.profilesByID(ctx, userIDs, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	responseProfiles := make([]*usersv1.UserProfile, 0, len(result.Hits))
	for _, hit := range result.Hits {
		userID := strings.TrimSpace(hit.UserID)
		if userID == "" {
			userID = strings.TrimSpace(hit.TargetID)
		}
		profile, ok := profiles[userID]
		if !ok {
			continue
		}
		responseProfiles = append(responseProfiles, profile)
	}

	return &usersv1.SearchUsersResponse{
		Profiles: responseProfiles,
		Page: &commonv1.PageResponse{
			NextPageToken: result.NextPageToken,
			TotalSize:     result.TotalSize,
		},
	}, nil
}

func (a *api) profileByID(ctx context.Context, accountID string, viewerID string) (*usersv1.UserProfile, error) {
	profiles, err := a.profilesByID(ctx, []string{accountID}, viewerID)
	if err != nil {
		return nil, err
	}

	userProfile, ok := profiles[accountID]
	if !ok {
		return nil, domainpresence.ErrNotFound
	}

	return userProfile, nil
}

func (a *api) profilesByID(
	ctx context.Context,
	accountIDs []string,
	viewerID string,
) (map[string]*usersv1.UserProfile, error) {
	if len(accountIDs) == 0 {
		return map[string]*usersv1.UserProfile{}, nil
	}

	presenceSnapshots, err := a.presence.ListPresence(ctx, accountIDs, viewerID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*usersv1.UserProfile, len(accountIDs))
	for _, accountID := range accountIDs {
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			continue
		}

		account, loadErr := a.identity.AccountByID(ctx, accountID)
		if loadErr != nil {
			continue
		}
		snapshot := presenceSnapshots[accountID]
		result[accountID] = profile(account, snapshot)
	}

	return result, nil
}
